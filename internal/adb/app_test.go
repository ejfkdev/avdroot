package adb

import (
	"os"
	"testing"
)

// The fixture is a real dump taken from an emulator running Magisk 31.0, with
// the "Requires additional setup" dialog on screen. Its value is that it is
// whatever uiautomator actually emitted, including the parts that are easy to
// guess wrong: the buttons carry no label and Compose reports them as neither
// clickable nor focused.
func loadFixture(t *testing.T) []UINode {
	t.Helper()
	data, err := os.ReadFile("testdata/ui-setup-dialog.xml")
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := ParseUIDump(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) == 0 {
		t.Fatal("the fixture parsed to no nodes")
	}
	return nodes
}

// The setup prompt must resolve to the button that proceeds, which is the
// rightmost one. Picking Cancel would leave Magisk half-installed with no
// obvious symptom, so this is the assertion that matters most.
func TestFindAffirmativeButtonPicksTheRightmost(t *testing.T) {
	nodes := loadFixture(t)

	button, ok := FindAffirmativeButton(nodes, MagiskPackage)
	if !ok {
		t.Fatal("no affirmative button was found in the setup dialog")
	}
	if got := ButtonLabel(nodes, button); got != "OK" {
		t.Errorf("found the button labelled %q, want OK", got)
	}
	if x, y := button.Bounds.Center(); x != 1113 || y != 1738 {
		t.Errorf("button centre = (%d, %d), want (1113, 1738)", x, y)
	}
}

func TestButtonLabelsReadsTheDialog(t *testing.T) {
	nodes := loadFixture(t)

	got := ButtonLabels(nodes, MagiskPackage)
	want := []string{"Cancel", "OK"}
	if len(got) != len(want) {
		t.Fatalf("labels = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("label %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// A dialog with a single button offers no choice, so pressing it cannot be
// assumed to mean "go ahead". This is what stops an error box from being
// answered blindly.
func TestFindAffirmativeButtonIgnoresALoneButton(t *testing.T) {
	xml := []byte(`<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
  <node class="android.widget.TextView" package="com.topjohnwu.magisk" text="Something failed" bounds="[10,10][500,80]"/>
  <node class="android.widget.Button" package="com.topjohnwu.magisk" text="" bounds="[800,100][1000,200]"/>
</hierarchy>`)
	nodes, err := ParseUIDump(xml)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := FindAffirmativeButton(nodes, MagiskPackage); ok {
		t.Error("a single button was treated as an affirmative choice")
	}
}

// Another app's dialog must never be answered, however it is laid out.
func TestFindAffirmativeButtonIgnoresOtherPackages(t *testing.T) {
	nodes := loadFixture(t)
	for _, pkg := range []string{"com.android.settings", "com.google.android.permissioncontroller", ""} {
		if _, ok := FindAffirmativeButton(nodes, pkg); ok {
			t.Errorf("a button was found for package %q", pkg)
		}
	}
}

func TestParseBounds(t *testing.T) {
	got, err := parseBounds("[798,1678][1002,1798]")
	if err != nil {
		t.Fatal(err)
	}
	want := Bounds{Left: 798, Top: 1678, Right: 1002, Bottom: 1798}
	if got != want {
		t.Errorf("parseBounds = %+v, want %+v", got, want)
	}
	if x, y := got.Center(); x != 900 || y != 1738 {
		t.Errorf("Center = (%d, %d), want (900, 1738)", x, y)
	}

	for _, bad := range []string{"", "[1,2][3]", "not-bounds", "[a,b][c,d]"} {
		if _, err := parseBounds(bad); err == nil {
			t.Errorf("parseBounds(%q) was accepted", bad)
		}
	}
}

// Bounds that cannot be parsed lose the tap target but must not lose the
// subtree beneath them.
func TestParseUIDumpKeepsChildrenOfUnusableNodes(t *testing.T) {
	xml := []byte(`<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
  <node class="android.widget.FrameLayout" bounds="">
    <node class="android.widget.Button" text="" bounds="[10,20][30,40]"/>
  </node>
</hierarchy>`)
	nodes, err := ParseUIDump(xml)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || !nodes[0].IsButton() {
		t.Fatalf("nodes = %+v, want the nested button only", nodes)
	}
}
