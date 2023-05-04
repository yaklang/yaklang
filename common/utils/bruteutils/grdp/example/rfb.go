// rfb.go
package main

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/glog"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/rfb"
)

type VncClient struct {
	Host   string // ip:port
	Width  int
	Height int
	vnc    *rfb.RFB
}

func NewVncClient(host string, width, height int, logLevel glog.LEVEL) *VncClient {
	return &VncClient{
		Host:   host,
		Width:  width,
		Height: height,
	}
}
func uiVnc(info *Info) (error, *VncClient) {
	BitmapCH = make(chan []Bitmap, 500)
	g := NewVncClient(fmt.Sprintf("%s:%s", info.Ip, info.Port), info.Width, info.Height, glog.INFO)

	g.Login()

	return nil, g
}

func (g *VncClient) Login() error {
	conn, err := net.DialTimeout("tcp", g.Host, 3*time.Second)
	if err != nil {
		return fmt.Errorf("[dial err] %v", err)
	}
	//defer conn.Close()
	g.vnc = rfb.NewRFB(rfb.NewRFBConn(conn))

	g.vnc.On("error", func(e error) {
		glog.Info("on error")
		glog.Error(e)
	}).On("close", func() {
		err = errors.New("close")
		glog.Info("on close")
	}).On("success", func() {
		err = nil
		glog.Info("on success")
	}).On("ready", func() {
		glog.Info("on ready")
	}).On("update", func(br *rfb.BitRect) {
		glog.Debug("on update:", br)
		bs := make([]Bitmap, 0, 50)
		for _, v := range br.Rects {
			b := Bitmap{int(v.Rect.X), int(v.Rect.Y), int(v.Rect.X + v.Rect.Width), int(v.Rect.Y + v.Rect.Height),
				int(v.Rect.Width), int(v.Rect.Height),
				Bpp(uint16(br.Pf.BitsPerPixel)), false, v.Data}
			bs = append(bs, b)
		}

		ui_paint_bitmap(bs)
	})
	return nil
}
func (g *VncClient) SetRequestedProtocol(p uint32) {

}
func (g *VncClient) KeyUp(sc int, name string) {
	glog.Debug("KeyUp:", sc, "name:", name)
	k := &rfb.KeyEvent{}
	k.Key = uint32(sc)
	g.vnc.SendKeyEvent(k)
}
func (g *VncClient) KeyDown(sc int, name string) {
	glog.Debug("KeyDown:", sc, "name:", name)
	k := &rfb.KeyEvent{}
	k.DownFlag = 1
	k.Key = uint32(sc)
	g.vnc.SendKeyEvent(k)
}

func (g *VncClient) MouseMove(x, y int) {
	if g == nil {
		return
	}
	glog.Info("MouseMove", x, ":", y)
	p := &rfb.PointerEvent{}
	time.Sleep(8 * time.Millisecond)
	p.XPos = uint16(x)
	p.YPos = uint16(y)
	g.vnc.SendPointEvent(p)
}

func (g *VncClient) MouseWheel(scroll, x, y int) {
	glog.Info("MouseWheel", x, ":", y)
}

func (g *VncClient) MouseUp(button int, x, y int) {
	glog.Info("MouseUp", x, ":", y, ":", button)
	p := &rfb.PointerEvent{}

	switch button {
	case 0:
		p.Mask = 1
	case 2:
		p.Mask = 1<<3 - 1
	case 1:
		p.Mask = 1<<2 - 1
	default:
		p.Mask = 0
	}
	p.XPos = uint16(x)
	p.YPos = uint16(y)
	g.vnc.SendPointEvent(p)
}
func (g *VncClient) MouseDown(button int, x, y int) {
	glog.Info("MouseDown:", x, ":", y, ":", button)
	p := &rfb.PointerEvent{}

	switch button {
	case 0:
		p.Mask = 1
	case 2:
		p.Mask = 1<<3 - 1
	case 1:
		p.Mask = 1<<2 - 1
	default:
		p.Mask = 0
	}

	p.XPos = uint16(x)
	p.YPos = uint16(y)
	g.MouseMove(x, y)
	g.vnc.SendPointEvent(p)
}

func (g *VncClient) Close() {
	if g.vnc != nil {
		g.vnc.Close()
	}
}

/*
# Modifier mask constants
MOD_SHIFT       = 1 << 0
MOD_CTRL        = 1 << 1
MOD_ALT         = 1 << 2
MOD_CAPSLOCK    = 1 << 3
MOD_NUMLOCK     = 1 << 4
MOD_WINDOWS     = 1 << 5
MOD_COMMAND     = 1 << 6
MOD_OPTION      = 1 << 7
MOD_SCROLLLOCK  = 1 << 8
MOD_FUNCTION    = 1 << 9

#: Accelerator modifier.  On Windows and Linux, this is ``MOD_CTRL``, on
#: Mac OS X it's ``MOD_COMMAND``.
MOD_ACCEL = MOD_CTRL
if compat_platform == 'darwin':
    MOD_ACCEL = MOD_COMMAND


# Key symbol constants

# ASCII commands
BACKSPACE     = 0xff08
TAB           = 0xff09
LINEFEED      = 0xff0a
CLEAR         = 0xff0b
RETURN        = 0xff0d
ENTER         = 0xff0d      # synonym
PAUSE         = 0xff13
SCROLLLOCK    = 0xff14
SYSREQ        = 0xff15
ESCAPE        = 0xff1b
SPACE         = 0xff20

# Cursor control and motion
HOME          = 0xff50
LEFT          = 0xff51
UP            = 0xff52
RIGHT         = 0xff53
DOWN          = 0xff54
PAGEUP        = 0xff55
PAGEDOWN      = 0xff56
END           = 0xff57
BEGIN         = 0xff58

# Misc functions
DELETE        = 0xffff
SELECT        = 0xff60
PRINT         = 0xff61
EXECUTE       = 0xff62
INSERT        = 0xff63
UNDO          = 0xff65
REDO          = 0xff66
MENU          = 0xff67
FIND          = 0xff68
CANCEL        = 0xff69
HELP          = 0xff6a
BREAK         = 0xff6b
MODESWITCH    = 0xff7e
SCRIPTSWITCH  = 0xff7e
FUNCTION      = 0xffd2

# Text motion constants: these are allowed to clash with key constants
MOTION_UP                = UP
MOTION_RIGHT             = RIGHT
MOTION_DOWN              = DOWN
MOTION_LEFT              = LEFT
MOTION_NEXT_WORD         = 1
MOTION_PREVIOUS_WORD     = 2
MOTION_BEGINNING_OF_LINE = 3
MOTION_END_OF_LINE       = 4
MOTION_NEXT_PAGE         = PAGEDOWN
MOTION_PREVIOUS_PAGE     = PAGEUP
MOTION_BEGINNING_OF_FILE = 5
MOTION_END_OF_FILE       = 6
MOTION_BACKSPACE         = BACKSPACE
MOTION_DELETE            = DELETE

# Number pad
NUMLOCK       = 0xff7f
NUM_SPACE     = 0xff80
NUM_TAB       = 0xff89
NUM_ENTER     = 0xff8d
NUM_F1        = 0xff91
NUM_F2        = 0xff92
NUM_F3        = 0xff93
NUM_F4        = 0xff94
NUM_HOME      = 0xff95
NUM_LEFT      = 0xff96
NUM_UP        = 0xff97
NUM_RIGHT     = 0xff98
NUM_DOWN      = 0xff99
NUM_PRIOR     = 0xff9a
NUM_PAGE_UP   = 0xff9a
NUM_NEXT      = 0xff9b
NUM_PAGE_DOWN = 0xff9b
NUM_END       = 0xff9c
NUM_BEGIN     = 0xff9d
NUM_INSERT    = 0xff9e
NUM_DELETE    = 0xff9f
NUM_EQUAL     = 0xffbd
NUM_MULTIPLY  = 0xffaa
NUM_ADD       = 0xffab
NUM_SEPARATOR = 0xffac
NUM_SUBTRACT  = 0xffad
NUM_DECIMAL   = 0xffae
NUM_DIVIDE    = 0xffaf

NUM_0         = 0xffb0
NUM_1         = 0xffb1
NUM_2         = 0xffb2
NUM_3         = 0xffb3
NUM_4         = 0xffb4
NUM_5         = 0xffb5
NUM_6         = 0xffb6
NUM_7         = 0xffb7
NUM_8         = 0xffb8
NUM_9         = 0xffb9

# Function keys
F1            = 0xffbe
F2            = 0xffbf
F3            = 0xffc0
F4            = 0xffc1
F5            = 0xffc2
F6            = 0xffc3
F7            = 0xffc4
F8            = 0xffc5
F9            = 0xffc6
F10           = 0xffc7
F11           = 0xffc8
F12           = 0xffc9
F13           = 0xffca
F14           = 0xffcb
F15           = 0xffcc
F16           = 0xffcd
F17           = 0xffce
F18           = 0xffcf
F19           = 0xffd0
F20           = 0xffd1

# Modifiers
LSHIFT        = 0xffe1
RSHIFT        = 0xffe2
LCTRL         = 0xffe3
RCTRL         = 0xffe4
CAPSLOCK      = 0xffe5
LMETA         = 0xffe7
RMETA         = 0xffe8
LALT          = 0xffe9
RALT          = 0xffea
LWINDOWS      = 0xffeb
RWINDOWS      = 0xffec
LCOMMAND      = 0xffed
RCOMMAND      = 0xffee
LOPTION       = 0xffef
ROPTION       = 0xfff0

# Latin-1
SPACE         = 0x020
EXCLAMATION   = 0x021
DOUBLEQUOTE   = 0x022
HASH          = 0x023
POUND         = 0x023  # synonym
DOLLAR        = 0x024
PERCENT       = 0x025
AMPERSAND     = 0x026
APOSTROPHE    = 0x027
PARENLEFT     = 0x028
PARENRIGHT    = 0x029
ASTERISK      = 0x02a
PLUS          = 0x02b
COMMA         = 0x02c
MINUS         = 0x02d
PERIOD        = 0x02e
SLASH         = 0x02f
_0            = 0x030
_1            = 0x031
_2            = 0x032
_3            = 0x033
_4            = 0x034
_5            = 0x035
_6            = 0x036
_7            = 0x037
_8            = 0x038
_9            = 0x039
COLON         = 0x03a
SEMICOLON     = 0x03b
LESS          = 0x03c
EQUAL         = 0x03d
GREATER       = 0x03e
QUESTION      = 0x03f
AT            = 0x040
BRACKETLEFT   = 0x05b
BACKSLASH     = 0x05c
BRACKETRIGHT  = 0x05d
ASCIICIRCUM   = 0x05e
UNDERSCORE    = 0x05f
GRAVE         = 0x060
QUOTELEFT     = 0x060
A             = 0x061
B             = 0x062
C             = 0x063
D             = 0x064
E             = 0x065
F             = 0x066
G             = 0x067
H             = 0x068
I             = 0x069
J             = 0x06a
K             = 0x06b
L             = 0x06c
M             = 0x06d
N             = 0x06e
O             = 0x06f
P             = 0x070
Q             = 0x071
R             = 0x072
S             = 0x073
T             = 0x074
U             = 0x075
V             = 0x076
W             = 0x077
X             = 0x078
Y             = 0x079
Z             = 0x07a
BRACELEFT     = 0x07b
BAR           = 0x07c
BRACERIGHT    = 0x07d
ASCIITILDE    = 0x07e
*/
