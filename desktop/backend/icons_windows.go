package backend

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                 = windows.NewLazySystemDLL("user32.dll")
	gdi32                  = windows.NewLazySystemDLL("gdi32.dll")
	procPrivateExtractIcon = user32.NewProc("PrivateExtractIconsW")
	procGetIconInfo        = user32.NewProc("GetIconInfo")
	procDestroyIcon        = user32.NewProc("DestroyIcon")
	procGetDC              = user32.NewProc("GetDC")
	procReleaseDC          = user32.NewProc("ReleaseDC")
	procGetDIBits          = gdi32.NewProc("GetDIBits")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
)

type iconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  windows.Handle
	hbmColor windows.Handle
}

type bitmapInfoHeader struct {
	size          uint32
	width         int32
	height        int32
	planes        uint16
	bitCount      uint16
	compression   uint32
	sizeImage     uint32
	xPelsPerMeter int32
	yPelsPerMeter int32
	clrUsed       uint32
	clrImportant  uint32
}

const iconSize = 64

// appIcon extracts an executable's main icon at 64 px and encodes it as
// PNG.
func appIcon(exe, name string) string {
	path, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return ""
	}
	var hicon windows.Handle
	var id uint32
	n, _, _ := procPrivateExtractIcon.Call(uintptr(unsafe.Pointer(path)), 0, iconSize, iconSize,
		uintptr(unsafe.Pointer(&hicon)), uintptr(unsafe.Pointer(&id)), 1, 0)
	if n == 0 || n == 0xFFFFFFFF || hicon == 0 {
		return ""
	}
	defer procDestroyIcon.Call(uintptr(hicon))
	var ii iconInfo
	if r, _, _ := procGetIconInfo.Call(uintptr(hicon), uintptr(unsafe.Pointer(&ii))); r == 0 {
		return ""
	}
	defer procDeleteObject.Call(uintptr(ii.hbmColor))
	defer procDeleteObject.Call(uintptr(ii.hbmMask))
	if ii.hbmColor == 0 {
		return ""
	}
	hdr := bitmapInfoHeader{width: iconSize, height: -iconSize, planes: 1, bitCount: 32} // top-down BGRA
	hdr.size = uint32(unsafe.Sizeof(hdr))
	pixels := make([]byte, iconSize*iconSize*4)
	dc, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, dc)
	if r, _, _ := procGetDIBits.Call(dc, uintptr(ii.hbmColor), 0, iconSize,
		uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&hdr)), 0); r == 0 {
		return ""
	}
	img := image.NewNRGBA(image.Rect(0, 0, iconSize, iconSize))
	hasAlpha := false
	for i := 0; i < len(pixels); i += 4 {
		if pixels[i+3] != 0 {
			hasAlpha = true
			break
		}
	}
	// Icons without an alpha channel keep their transparency in the AND
	// mask: white where the background shows through.
	var mask []byte
	if !hasAlpha && ii.hbmMask != 0 {
		mask = make([]byte, iconSize*iconSize*4)
		mhdr := bitmapInfoHeader{width: iconSize, height: -iconSize, planes: 1, bitCount: 32}
		mhdr.size = uint32(unsafe.Sizeof(mhdr))
		if r, _, _ := procGetDIBits.Call(dc, uintptr(ii.hbmMask), 0, iconSize,
			uintptr(unsafe.Pointer(&mask[0])), uintptr(unsafe.Pointer(&mhdr)), 0); r == 0 {
			mask = nil
		}
	}
	for y := 0; y < iconSize; y++ {
		for x := 0; x < iconSize; x++ {
			o := (y*iconSize + x) * 4
			a := pixels[o+3]
			if !hasAlpha {
				a = 255
				if mask != nil && mask[o] != 0 {
					a = 0
				}
			}
			img.SetNRGBA(x, y, color.NRGBA{R: pixels[o+2], G: pixels[o+1], B: pixels[o], A: a})
		}
	}
	var buf bytes.Buffer
	if png.Encode(&buf, img) != nil {
		return ""
	}
	return dataURL("image/png", buf.Bytes())
}
