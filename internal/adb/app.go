package adb

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// MagiskPackage is the Magisk manager app. It matters beyond being an app the
// user might open: patching the ramdisk gives Magisk an init and a daemon, but
// the rest of its files live in this package and are only unpacked once the app
// has run its first-start setup.
const MagiskPackage = "com.topjohnwu.magisk"

// permissionPostNotifications is the single runtime permission the Magisk app
// asks for, and it asks before it will do anything else. Granting it up front
// keeps the permission dialog out of the way of the setup prompt.
const permissionPostNotifications = "android.permission.POST_NOTIFICATIONS"

// MagiskEnvBinary marks that the manager app has unpacked its payload into
// /data/adb/magisk. Patching the ramdisk does not create it; the app's setup
// does, and until then the "su" on PATH is the emulator's own su.
const MagiskEnvBinary = "/data/adb/magisk/magisk"

// uiDumpPath is where "uiautomator dump" is told to write. It is on the sdcard
// so that no root is needed to read it back.
const uiDumpPath = "/sdcard/avdroot-ui.xml"

// Bounds is a rectangle in screen pixels, in the form uiautomator reports.
type Bounds struct {
	Left, Top, Right, Bottom int
}

// Center is the point to tap to press whatever these bounds describe.
func (b Bounds) Center() (x, y int) { return (b.Left + b.Right) / 2, (b.Top + b.Bottom) / 2 }

// Contains reports whether the centre of inner lies within b. Nested nodes are
// how a label is matched to the button that carries it.
func (b Bounds) Contains(inner Bounds) bool {
	x, y := inner.Center()
	return x >= b.Left && x <= b.Right && y >= b.Top && y <= b.Bottom
}

// UINode is one element of a uiautomator dump, flattened out of the tree.
type UINode struct {
	Text    string
	Class   string
	Package string
	Bounds  Bounds
}

// IsButton reports whether the node is a button. Compose reports neither a
// clickable flag nor a label on the button itself, so the class is what is left
// to go on; the label is found separately, from the text drawn inside it.
func (n UINode) IsButton() bool { return n.Class == "android.widget.Button" }

// uiXML is the document "uiautomator dump" writes. The hierarchy nests to an
// arbitrary depth, so the node type refers to itself.
type uiXML struct {
	Nodes []uiXMLNode `xml:"node"`
}

type uiXMLNode struct {
	Text     string      `xml:"text,attr"`
	Class    string      `xml:"class,attr"`
	Package  string      `xml:"package,attr"`
	Bounds   string      `xml:"bounds,attr"`
	Children []uiXMLNode `xml:"node"`
}

// ParseUIDump turns the XML from "uiautomator dump" into a flat node list in
// document order. That order is load-bearing: Material puts the affirmative
// button of a dialog last, and the caller relies on it.
func ParseUIDump(data []byte) ([]UINode, error) {
	var doc uiXML
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing the uiautomator dump: %w", err)
	}
	var out []UINode
	for _, n := range doc.Nodes {
		flattenUIDump(n, &out)
	}
	return out, nil
}

func flattenUIDump(n uiXMLNode, out *[]UINode) {
	b, err := parseBounds(n.Bounds)
	if err != nil {
		// A node without usable bounds cannot be tapped, but its children can.
		for _, c := range n.Children {
			flattenUIDump(c, out)
		}
		return
	}
	*out = append(*out, UINode{Text: n.Text, Class: n.Class, Package: n.Package, Bounds: b})
	for _, c := range n.Children {
		flattenUIDump(c, out)
	}
}

// parseBounds reads the "[left,top][right,bottom]" form.
func parseBounds(s string) (Bounds, error) {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '[' || r == ']' || r == ','
	})
	if len(parts) != 4 {
		return Bounds{}, fmt.Errorf("unexpected bounds %q", s)
	}
	var v [4]int
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return Bounds{}, fmt.Errorf("unexpected bounds %q: %w", s, err)
		}
		v[i] = n
	}
	return Bounds{Left: v[0], Top: v[1], Right: v[2], Bottom: v[3]}, nil
}

// FindAffirmativeButton returns the button that accepts a dialog shown by pkg.
//
// The button cannot be found by its label: Magisk ships ninety translations and
// the emulator may be in any of them. What does hold is that a Material dialog
// lays its buttons out in a row and puts the affirmative one last, so it is the
// rightmost. Requiring at least two buttons is what keeps a lone dismiss button
// from being mistaken for an invitation to proceed.
func FindAffirmativeButton(nodes []UINode, pkg string) (UINode, bool) {
	var buttons []UINode
	for _, n := range nodes {
		if n.Package == pkg && n.IsButton() {
			buttons = append(buttons, n)
		}
	}
	if len(buttons) < 2 {
		return UINode{}, false
	}
	best := buttons[0]
	bx, _ := best.Bounds.Center()
	for _, b := range buttons[1:] {
		if x, _ := b.Bounds.Center(); x > bx {
			best, bx = b, x
		}
	}
	return best, true
}

// ButtonLabel returns the text drawn inside a button, for reporting what was
// clicked. Compose leaves the button itself unlabelled, so the label is
// whichever text node sits inside it.
func ButtonLabel(nodes []UINode, button UINode) string {
	for _, n := range nodes {
		if n.Class == "android.widget.TextView" && n.Text != "" && button.Bounds.Contains(n.Bounds) {
			return n.Text
		}
	}
	return ""
}

// ButtonLabels lists every button label on screen for pkg, in document order.
func ButtonLabels(nodes []UINode, pkg string) []string {
	var labels []string
	for _, n := range nodes {
		if n.Package == pkg && n.IsButton() {
			if l := ButtonLabel(nodes, n); l != "" {
				labels = append(labels, l)
			}
		}
	}
	return labels
}

// UIDump returns the current screen's node list.
func (c *Client) UIDump() ([]UINode, error) {
	// The tool prints "UI hierchary dumped to: ..." on the same stream as the
	// document, so the file is written quietly and then read back.
	if _, err := c.Shellf("uiautomator dump %s >/dev/null 2>&1", uiDumpPath); err != nil {
		return nil, err
	}
	out, err := c.Shell("cat " + uiDumpPath)
	if err != nil {
		return nil, err
	}
	nodes, err := ParseUIDump([]byte(out))
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, errors.New("uiautomator returned an empty hierarchy")
	}
	return nodes, nil
}

// Tap presses the screen at x, y.
func (c *Client) Tap(x, y int) error {
	_, err := c.Shellf("input tap %d %d", x, y)
	return err
}

// ForegroundPackage names the app currently in front. It is what keeps a tap
// aimed at a dialog the emulator owns rather than at whatever happened to be on
// screen.
func (c *Client) ForegroundPackage() (string, error) {
	out, err := c.Shell("dumpsys activity activities | grep -m1 topResumedActivity")
	if err != nil {
		return "", err
	}
	// The line looks like:
	//   topResumedActivity=ActivityRecord{... u0 com.example/.Main t9}
	const marker = " u0 "
	i := strings.Index(out, marker)
	if i < 0 {
		return "", fmt.Errorf("cannot read the foreground app from %q", strings.TrimSpace(out))
	}
	rest := out[i+len(marker):]
	if j := strings.IndexAny(rest, " /"); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSpace(rest), nil
}

// AllowNotifications grants the Magisk app the one runtime permission it asks
// for, so that the prompt for it does not stand in front of the setup dialog.
//
// Granting silently is what makes the step deterministic rather than merely
// convenient: a prompt left on screen would have to be answered by guessing at
// a label in one of ninety languages.
func (c *Client) AllowNotifications(pkg string) error {
	_, err := c.Shellf("pm grant %s %s", pkg, permissionPostNotifications)
	return err
}

// LaunchApp starts an app's launcher activity.
//
// "monkey" is used rather than "am start" because it needs no activity name,
// which would otherwise have to be hard-coded per app version.
func (c *Client) LaunchApp(pkg string) error {
	_, err := c.Shellf("monkey -p %s -c android.intent.category.LAUNCHER 1 >/dev/null 2>&1", pkg)
	return err
}

// BootID is a fresh random value on every boot, which makes it a reliable way
// to tell a reboot apart from the device merely being busy.
func (c *Client) BootID() (string, error) {
	out, err := c.Shell("cat /proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(out)
	if id == "" {
		return "", errors.New("the device reported no boot id")
	}
	return id, nil
}

// MagiskEnvInstalled reports whether the manager app has unpacked Magisk into
// /data/adb/magisk. Reading it needs root, and gaining root is the caller's
// business, so this leaves adbd as it found it only in the sense that it does
// not switch it back.
//
// The shell test is written to exit 0 either way: a missing file is the answer
// to the question, not a failure to ask it.
func (c *Client) MagiskEnvInstalled() (bool, error) {
	if err := c.EnsureRoot(); err != nil {
		return false, err
	}
	out, err := c.Shellf("if [ -x %s ]; then echo yes; else echo no; fi", MagiskEnvBinary)
	if err != nil {
		return false, err
	}
	return strings.Contains(out, "yes"), nil
}
