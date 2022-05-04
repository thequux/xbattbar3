package main

import (
	"flag"
	"fmt"
	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgbutil"
	"github.com/BurntSushi/xgbutil/ewmh"
	"github.com/BurntSushi/xgbutil/icccm"
	"github.com/BurntSushi/xgbutil/motif"
	"os"
	"strings"
	"time"
)

var XU *xgbutil.XUtil
var X *xgb.Conn
var screen *xproto.ScreenInfo
var bar_win xproto.Window
var popup_win xproto.Window
var horizontal bool
var bar_length uint16
var bar_width uint16
var popup_visible = false

var current_state *PowerStatus

type ScreenSide int

const (
	LEFT ScreenSide = iota
	TOP
	RIGHT
	BOTTOM
)

var checker_flag = flag.String("checker", "upower", "The checker to use. Some checkers may require arguments; these may be given after a ':'")
var update_freq = flag.Duration("r", 5*time.Second, "Time between updates")
var side = BOTTOM

func (s ScreenSide) String() string {
	switch s {
	case LEFT:
		return "left"
	case TOP:
		return "top"
	case RIGHT:
		return "right"
	case BOTTOM:
		return "bottom"
	}
	return "unknown"
}

func (s *ScreenSide) Set(v string) error {
	switch strings.ToLower(v) {
	case "left", "l":
		*s = LEFT
	case "right", "r":
		*s = RIGHT
	case "top", "t":
		*s = TOP
	case "bottom", "b":
		*s = BOTTOM
	default:
		return fmt.Errorf("Valid sides: l,r,t,b,left,right,top,bottom")
	}
	return nil
}

var (
	color_black          uint32 = 0xFF000000
	color_white          uint32 = 0xFFFFFFFF
	color_charging_bg    uint32 = 0xFF004488
	color_charging_fg    uint32 = 0xFF0088FF
	color_discharging_bg uint32 = 0xFF880000
	color_discharging_fg uint32 = 0xFF0088FF
	gc                   xproto.Gcontext
)

// Atoms
var (
	NET_WM_STRUT_PARTIAL xproto.Atom
	NET_WM_STRUT         xproto.Atom
	CARDINAL             xproto.Atom
)

var stderr = os.Stderr

func MustGC(name string, cookie xproto.CreateGCCookie) {
	if err := cookie.Check(); err != nil {
		fmt.Fprintln(stderr,
			"Failed to allocate GC ",
			name, ": ", err)
		os.Exit(1)
	}
}

type PendingAtom struct {
	dest   *xproto.Atom
	name   string
	cookie xproto.InternAtomCookie
}

type Atomizer []PendingAtom

func (a *Atomizer) Intern(name string, atom *xproto.Atom) {
	*a = append(*a, PendingAtom{
		dest:   atom,
		name:   name,
		cookie: xproto.InternAtom(X, false, uint16(len(name)), name),
	})
}

func (a *Atomizer) Flush() error {
	defer func() {
		*a = (*a)[:0]
	}()
	for _, p := range *a {
		if reply, err := p.cookie.Reply(); err != nil {
			return err
		} else {
			*p.dest = reply.Atom
		}
	}
	return nil
}

func main() {
	flag.Var(&side, "side", "Which side of the screen to put the bar on (l,r,t,b)")
	bar_width_f := flag.Int("w", 3, "Thickness of bar")
	flag.Parse()
	bar_width = uint16(*bar_width_f)

	var err error
	XU, err = xgbutil.NewConn()
	if err != nil {
		fmt.Fprintln(stderr, "Failed to open connection: ", err)
		return
	}
	X = XU.Conn()

	setup := xproto.Setup(X)
	screen = setup.DefaultScreen(X)

	var bar_x, bar_y int16
	var bar_w, bar_h uint16

	switch side {
	case TOP:
		bar_x, bar_y = 0, 0
		bar_w, bar_h = screen.WidthInPixels, bar_width
		horizontal = true
	case BOTTOM:
		bar_x, bar_y = 0, int16(screen.HeightInPixels-bar_width)
		bar_w, bar_h = screen.WidthInPixels, bar_width
		horizontal = true
	case LEFT:
		bar_x, bar_y = 0, 0
		bar_w, bar_h = bar_width, screen.HeightInPixels
		horizontal = false
	case RIGHT:
		bar_x, bar_y = int16(screen.WidthInPixels-bar_width), 0
		bar_w, bar_h = bar_width, screen.HeightInPixels
		horizontal = false
	}
	if horizontal {
		bar_length = screen.WidthInPixels
	} else {
		bar_length = screen.HeightInPixels
	}

	bar_win, _ = xproto.NewWindowId(X)
	popup_win, _ = xproto.NewWindowId(X)

	xproto.CreateWindow(X, screen.RootDepth, bar_win, screen.Root,
		bar_x, bar_y, bar_w, bar_h, 0,
		xproto.WindowClassInputOutput,
		screen.RootVisual,
		xproto.CwBackPixel|
			xproto.CwEventMask,
		[]uint32{
			0xFF00FF00,
			xproto.EventMaskStructureNotify |
				xproto.EventMaskKeyPress |
				xproto.EventMaskKeyRelease |
				xproto.EventMaskEnterWindow |
				xproto.EventMaskLeaveWindow |
				xproto.EventMaskExposure,
		})

	xproto.CreateWindow(X, screen.RootDepth, popup_win, screen.Root,
		int16(screen.WidthInPixels/2), int16(screen.HeightInPixels/2),
		50, 50, 1,
		xproto.WindowClassInputOutput,
		screen.RootVisual,
		xproto.CwBackPixel|
			xproto.CwWinGravity|
			xproto.CwOverrideRedirect,
		[]uint32{
			0xFFFFFFFF,
			xproto.GravityCenter,
			1,
		})

	// Set EWMH struts
	atomizer := new(Atomizer)
	atomizer.Intern("_NET_WM_STRUT_PARTIAL", &NET_WM_STRUT_PARTIAL)
	atomizer.Intern("_NET_WM_STRUT", &NET_WM_STRUT)
	atomizer.Intern("CARDINAL", &CARDINAL)
	if err := atomizer.Flush(); err != nil {
		fmt.Fprintln(stderr, "Failed to intern atoms: ", err)
		return
	}

	switch side {
	case BOTTOM:
		ewmh.WmStrutPartialSet(XU, bar_win, &ewmh.WmStrutPartial{
			0, 0, 0, uint(bar_width),
			0, 0, 0, 0, 0, 0, 0, uint(screen.WidthInPixels),
		})
		ewmh.WmStrutSet(XU, bar_win, &ewmh.WmStrut{
			0, 0, 0, uint(bar_width),
		})
	case TOP:
		ewmh.WmStrutPartialSet(XU, bar_win, &ewmh.WmStrutPartial{
			0, 0, uint(bar_width), 0,
			0, 0, 0, 0, 0, uint(screen.WidthInPixels), 0, 0,
		})
		ewmh.WmStrutSet(XU, bar_win, &ewmh.WmStrut{
			0, 0, uint(bar_width), 0,
		})
	case LEFT:
		/*ewmh.WmStrutPartialSet(XU, bar_win, &ewmh.WmStrutPartial{
			uint(bar_width), 0, 0, 0,
			0, uint(screen.HeightInPixels), 0, 0, 0, 0, 0, 0,
		})*/
		ewmh.WmStrutSet(XU, bar_win, &ewmh.WmStrut{
			uint(bar_width), 0, 0, 0,
		})
	case RIGHT:
		ewmh.WmStrutPartialSet(XU, bar_win, &ewmh.WmStrutPartial{
			0, uint(bar_width), 0, 0,
			0, 0, 0, uint(screen.HeightInPixels), 0, 0, 0, 0,
		})
		ewmh.WmStrutSet(XU, bar_win, &ewmh.WmStrut{
			0, uint(bar_width), 0, 0,
		})

	}

	ewmh.WmStateSet(XU, bar_win, []string{"_NET_WM_STATE_STICKY", "_NET_WM_STATE_ABOVE"})

	motif.WmHintsSet(XU, bar_win, &motif.Hints{
		Flags:      motif.HintDecorations,
		Decoration: motif.DecorationNone,
	})

	ewmh.WmWindowTypeSet(XU, bar_win, []string{"_NET_WM_WINDOW_TYPE_DOCK"})
	icccm.WmHintsSet(XU, bar_win, &icccm.Hints{
		Flags: icccm.HintInput,
		Input: 0,
	})
	/*
		icccm.WmNormalHintsSet(XU, bar_win, &icccm.NormalHints{
			Flags: icccm.SizeHintPSize |
				icccm.SizeHintPPosition |
				icccm.SizeHintPMaxSize |
				icccm.SizeHintPMinSize |
				icccm.SizeHintPWinGravity,
			X:          int(bar_x),
			Y:          int(bar_y),
			Width:      uint(bar_w),
			Height:     uint(bar_h),
			MinWidth:   uint(bar_w),
			MaxWidth:   uint(bar_w),
			MinHeight:  uint(bar_h),
			MaxHeight:  uint(bar_h),
			WinGravity: xproto.GravitySouthWest,
		})
	*/

	err = xproto.MapWindowChecked(X, bar_win).Check()
	if err != nil {
		fmt.Fprintln(stderr, "Failed to map window: ", err)
		return
	}

	font, _ := xproto.NewFontId(X)
	font_name := "-misc-fixed-medium-r-*-*-20-*-*-*-*-*-iso8859-1"
	// font_name := "-*-terminus-medium-r-*-*-18-*-*-*-*-*-iso8859-*"
	xproto.OpenFont(X, font, uint16(len(font_name)), font_name)

	gc, _ = xproto.NewGcontextId(X)
	MustGC("gc", xproto.CreateGCChecked(X, gc,
		xproto.Drawable(screen.Root),
		xproto.GcForeground|xproto.GcFont,
		[]uint32{
			0xFF000000,
			uint32(font),
		}))
	xproto.CloseFont(X, font)

	X.Sync()
	go UpdateProc()
	// Event loop...
	for {
		ev, err := X.WaitForEvent()
		if ev == nil && err == nil {
			fmt.Fprintln(stderr, "Both event and error are nil. This should never happen")
			return
		} else if err != nil {
			fmt.Println("Error: ", err)
			continue
		}

		switch ev.(type) {
		case xproto.EnterNotifyEvent:
			ShowPopup()
		case xproto.LeaveNotifyEvent:
			HidePopup()
		case xproto.MapNotifyEvent:
			DrawBar(current_state)
		case xproto.ExposeEvent:
			DrawBar(current_state)
			//default:
			//	fmt.Println("Event: ", ev)
		}

	}
}

type PowerStatus struct {
	ChargeLevel   float32 // between 0 and 1
	TimeRemaining float32 // in seconds, NaN if unavailable
	Charging      bool
}

type CheckerBackend interface {
	Init(args string) error
	Check() (*PowerStatus, error)
	Stop()
}

func string2c2b(text string) (res []xproto.Char2b) {
	res = make([]xproto.Char2b, 0, len(text))
	for _, c := range text {
		// hope that the character is in range...
		res = append(res, xproto.Char2b{byte(c >> 8), byte(c)})
	}
	return
}

type RGBA struct {
	R, G, B, A byte
}

func (rgba RGBA) Uint32() uint32 {
	return (uint32(rgba.A)<<24 |
		uint32(rgba.R)<<16 |
		uint32(rgba.G)<<8 |
		uint32(rgba.B))
}

func (a RGBA) Lerp(b RGBA, r float32) RGBA {
	ir := 1 - r
	return RGBA{
		byte(float32(a.R)*ir + float32(b.R)*r),
		byte(float32(a.G)*ir + float32(b.G)*r),
		byte(float32(a.B)*ir + float32(b.B)*r),
		byte(float32(a.A)*ir + float32(b.A)*r),
	}
}

var (
	ColorEmpty Oklab = RGB{1, 0, 0}.ToOklab()
	ColorMid   Oklab = RGB{1, 1, 0}.ToOklab()
	ColorFull  Oklab = RGB{0, 1, 0}.ToOklab()
)

func DrawBar(status *PowerStatus) {
	var drawAmt uint16
	if status == nil {
		fg := RGBA{128, 128, 128, 255}.Uint32()

		xproto.ChangeGC(X, gc,
			xproto.GcForeground,
			[]uint32{fg})

		if horizontal {
			xproto.PolyFillRectangle(X, xproto.Drawable(bar_win), gc,
				[]xproto.Rectangle{
					{0, 0, bar_length, bar_width},
				})
		} else {
			xproto.PolyFillRectangle(X, xproto.Drawable(bar_win), gc,
				[]xproto.Rectangle{
					{0, 0, bar_width, bar_length},
				})
		}
		return
	}
	if status.ChargeLevel >= 1 {
		drawAmt = bar_length
	} else if status.ChargeLevel <= 0 {
		drawAmt = 0
	} else {
		drawAmt = uint16(float32(bar_length) * status.ChargeLevel)
	}

	var fg, bg uint32
	if status.Charging {
		fg, bg = color_charging_fg, color_charging_bg
	} else {
		var fg_oklab, bg_oklab Oklab
		/*
			if status.ChargeLevel < 0.5 {
				fg_oklab = ColorEmpty.Lerp(
					ColorMid,
					status.ChargeLevel*2)
			} else {
				fg_oklab = ColorFull.Lerp(
					ColorMid,
					(1-status.ChargeLevel)*2)
			}
		*/

		fg_oklab = ColorEmpty.Lerp(ColorFull, status.ChargeLevel)

		bg_oklab = fg_oklab
		bg_oklab.L = bg_oklab.L * 0.7
		fg_rgba := fg_oklab.ToRGB()
		fg = fg_rgba.ToRGBA(255).Uint32()
		bg_rgba := bg_oklab.ToRGB()
		bg = bg_rgba.ToRGBA(255).Uint32()
		//fmt.Fprintf(stderr, "FG: %#v  bg: %v\n", ColorFull, bg_rgba)
	}

	xproto.ChangeGC(X, gc,
		xproto.GcForeground,
		[]uint32{fg})
	if horizontal {
		xproto.PolyFillRectangle(X, xproto.Drawable(bar_win), gc,
			[]xproto.Rectangle{
				{0, 0, drawAmt, bar_width},
			})
	} else {
		xproto.PolyFillRectangle(X, xproto.Drawable(bar_win), gc,
			[]xproto.Rectangle{
				{0, int16(bar_length - drawAmt), bar_width, drawAmt},
			})
	}
	xproto.ChangeGC(X, gc,
		xproto.GcForeground,
		[]uint32{bg})
	if horizontal {
		xproto.PolyFillRectangle(X, xproto.Drawable(bar_win), gc,
			[]xproto.Rectangle{
				{int16(drawAmt), 0, bar_length - drawAmt, bar_width},
			})
	} else {
		xproto.PolyFillRectangle(X, xproto.Drawable(bar_win), gc,
			[]xproto.Rectangle{
				{0, 0, bar_width, uint16(bar_length - drawAmt)},
			})
	}

	if popup_visible {
		xproto.ChangeGC(X, gc,
			xproto.GcForeground|xproto.GcBackground,
			[]uint32{color_black, color_white})

		windowContent := fmt.Sprintf("Charge level: %d%%",
			int(status.ChargeLevel*100+0.5))
		contentc2b := string2c2b(windowContent)

		extents, err := xproto.QueryTextExtents(X, xproto.Fontable(gc),
			contentc2b, uint16(len(contentc2b))).Reply()
		if err != nil {
			fmt.Fprintln(stderr, "Error measuring text: ", err)
			return
		}

		pop_width := uint32(extents.OverallWidth + 10)
		pop_height := uint32(extents.FontAscent + extents.FontDescent + 10)
		pop_x := (uint32(screen.WidthInPixels) - pop_width) / 2
		pop_y := (uint32(screen.HeightInPixels) - pop_height) / 2
		xproto.ConfigureWindow(X, popup_win,
			xproto.ConfigWindowX|
				xproto.ConfigWindowY|
				xproto.ConfigWindowWidth|
				xproto.ConfigWindowHeight,
			[]uint32{pop_x, pop_y, pop_width, pop_height})

		xproto.ImageText16(X, byte(len(contentc2b)),
			xproto.Drawable(popup_win),
			gc, 5, 5+extents.FontAscent, contentc2b)
	}

}

func UpdateProc() {
	var checker CheckerBackend

	checker_parts := strings.SplitN(*checker_flag, ":", 2)
	if len(checker_parts) < 2 {
		checker_parts = append(checker_parts, "", "")
	}
	switch checker_parts[0] {
	case "upower":
		checker = &UPowerChecker{}
	case "debug":
		checker = &DebugChecker{}
	default:
		panic("Unknown checker " + checker_parts[0])
	}
	if err := checker.Init(checker_parts[1]); err != nil {
		panic(err)
	}

	for {
		status, err := checker.Check()
		current_state = status
		if err != nil {
			fmt.Fprintf(stderr,
				"Failed to check battery level: %s", err)
		} else {
			//fmt.Fprintf(stderr, "Current status: %v", status)
			DrawBar(status)
		}
		time.Sleep(*update_freq)
	}
}

func ShowPopup() {
	if popup_visible {
		return
	}
	err := xproto.MapWindowChecked(X, popup_win).Check()
	if err != nil {
		fmt.Println(stderr, "Failed to map popup: ", err)
		return
	}

	popup_visible = true
	DrawBar(current_state)
}

func HidePopup() {
	if !popup_visible {
		return
	}
	xproto.UnmapWindow(X, popup_win)
	popup_visible = false
}
