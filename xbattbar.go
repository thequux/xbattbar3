package main

import (
	"fmt"
	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgbutil"
	"github.com/BurntSushi/xgbutil/ewmh"
	"os"
	"time"
	"github.com/BurntSushi/xgbutil/motif"
	"github.com/BurntSushi/xgbutil/icccm"
)

var XU *xgbutil.XUtil
var X *xgb.Conn
var screen *xproto.ScreenInfo
var bar_win xproto.Window
var popup_win xproto.Window
var horizontal bool
var bar_length uint16
var bar_width uint16 = 3
var (
	color_black          uint32 = 0xFF000000
	color_white          uint32 = 0xFFFFFFFF
	color_charging_bg    uint32 = 0xFF008800
	color_charging_fg    uint32 = 0xFF00FF00
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
	var err error
	XU, err = xgbutil.NewConn()
	if err != nil {
		fmt.Fprintln(stderr, "Failed to open connection: ", err)
		return
	}
	X = XU.Conn()

	setup := xproto.Setup(X)
	screen = setup.DefaultScreen(X)

	bar_length = screen.WidthInPixels

	bar_win, _ = xproto.NewWindowId(X)
	popup_win, _ = xproto.NewWindowId(X)
	xproto.CreateWindow(X, screen.RootDepth, bar_win, screen.Root,
		0, int16(screen.HeightInPixels-bar_width),
		screen.WidthInPixels, 3, 0,
		xproto.WindowClassInputOutput,
		screen.RootVisual,
		xproto.CwBackPixel|
			xproto.CwEventMask|
			xproto.CwOverrideRedirect,
		[]uint32{
			0xFF00FF00,
			0,
			xproto.EventMaskStructureNotify |
				xproto.EventMaskKeyPress |
				xproto.EventMaskKeyRelease,
		})

	xproto.CreateWindow(X, screen.RootDepth, popup_win, screen.Root,
		0, 0, 50, 50, 1,
		xproto.WindowClassInputOutput,
		screen.RootVisual,
		xproto.CwBackPixel|
			xproto.CwOverrideRedirect,
		[]uint32{
			0xFFFFFFFF,
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

	ewmh.WmStrutPartialSet(XU, bar_win, &ewmh.WmStrutPartial{
		0, 0, 0, uint(bar_width),
		0, 0, 0, 0, 0, 0, 0, uint(screen.WidthInPixels),
	})
	ewmh.WmStrutSet(XU, bar_win, &ewmh.WmStrut{
		0, 0, 0, uint(bar_width),
	})

	ewmh.WmStateSet(XU, bar_win, []string{"_NET_WM_STATE_STICKY", "_NET_WM_STATE_ABOVE"})

	motif.WmHintsSet(XU, bar_win, &motif.Hints{
		Flags: motif.HintDecorations,
		Decoration: motif.DecorationNone,
	})

	ewmh.WmWindowTypeSet(XU, bar_win, []string{"_NET_WM_WINDOW_TYPE_DOCK"})
	icccm.WmHintsSet(XU, bar_win, &icccm.Hints{
		Flags: icccm.HintInput,
		Input: 0,
	})
	icccm.WmNormalHintsSet(XU, bar_win, &icccm.NormalHints{
		Flags: icccm.SizeHintPSize |
			icccm.SizeHintPPosition |
			icccm.SizeHintPMaxSize |
			icccm.SizeHintPMinSize |
			icccm.SizeHintPWinGravity,
		X: 0,
		Y: int(screen.HeightInPixels-bar_width),
		Width: uint(screen.WidthInPixels),
		Height: uint(bar_width),
		MinWidth: uint(screen.WidthInPixels),
		MaxWidth: uint(screen.WidthInPixels),
		MinHeight: uint(screen.HeightInPixels),
		MaxHeight: uint(screen.HeightInPixels),
		WinGravity: xproto.GravitySouthWest,
		
	})
	
	err = xproto.MapWindowChecked(X, bar_win).Check()
	if err != nil {
		fmt.Fprintln(stderr, "Failed to map window: ", err)
		return
	}

	/*
	err = xproto.MapWindowChecked(X, popup_win).Check()
	if err != nil {
		fmt.Println(stderr, "Failed to map popup: ", err)
		return
	}*/

	gc, _ = xproto.NewGcontextId(X)
	MustGC("gc", xproto.CreateGCChecked(X, gc,
		xproto.Drawable(screen.Root),
		xproto.GcForeground,
		[]uint32{
			0xFF000000,
		}))

	go UpdateProc()
	// Event loop...
	for {
		ev, err := X.WaitForEvent()
		if ev == nil && err == nil {
			fmt.Fprintln(stderr, "Both event and error are nil. This should never happen")
			return
		} else if ev != nil {
			fmt.Println("Event: ", ev)
		} else if err != nil {
			fmt.Println("Error: ", err)
		}

	}
}

type PowerStatus struct {
	ChargeLevel   float32 // between 0 and 1
	TimeRemaining float32 // in seconds, NaN if unavailable
	Charging      bool
}

type CheckerBackend interface {
	Init() error
	Check() (*PowerStatus, error)
	Stop()
}

func DrawBar(status *PowerStatus) {
	var drawAmt uint16
	if status.ChargeLevel >= 1 {
		drawAmt = bar_length
	} else if status.ChargeLevel <= 0 {
		drawAmt = 0
	} else {
		drawAmt = uint16(float32(screen.WidthInPixels) *
			status.ChargeLevel)
	}

	var fg, bg uint32
	if status.Charging {
		fg, bg = color_charging_fg, color_charging_bg
	} else {
		fg, bg = color_discharging_fg, color_discharging_bg
	}

	xproto.ChangeGC(X, gc,
		xproto.GcForeground,
		[]uint32{fg})
	xproto.PolyFillRectangle(X, xproto.Drawable(bar_win), gc,
		[]xproto.Rectangle{
			{0, 0, drawAmt, bar_width},
		})
	xproto.ChangeGC(X, gc,
		xproto.GcForeground,
		[]uint32{bg})
	xproto.PolyFillRectangle(X, xproto.Drawable(bar_win), gc,
		[]xproto.Rectangle{
			{int16(drawAmt), 0, bar_length - drawAmt, bar_width},
		})
	var _ = drawAmt
}

func UpdateProc() {
	checker := &UPowerChecker{}
	if err := checker.Init(); err != nil {
		panic(err)
	}

	for {
		status, err := checker.Check()
		if err != nil {
			fmt.Fprintf(stderr,
				"Failed to check battery level: %s", err)
		} else {
			fmt.Printf("Drawing %v\n", status)
			DrawBar(status)
		}
		time.Sleep(5 * time.Second)
	}
}
