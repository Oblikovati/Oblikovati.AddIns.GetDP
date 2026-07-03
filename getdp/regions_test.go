// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"strings"
	"testing"
)

func TestRegionTableAllocatesSequentialTags(t *testing.T) {
	mesh := oneTetMesh()
	regions := newRegionTable([]string{"Coil", "Core"})
	if got, _ := regions.VolumeTag(0); got != 1 {
		t.Errorf("body 0 tag = %d, want 1", got)
	}
	if got, _ := regions.VolumeTag(1); got != 2 {
		t.Errorf("body 1 tag = %d, want 2", got)
	}
	tag, err := regions.BindSurface("electrode", []string{"k"}, fakeGroups(mesh, "k"))
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	if tag != 3 {
		t.Errorf("first surface tag = %d, want 3 (after the 2 volumes)", tag)
	}
}

func TestRegionTableBindSurfaceUnknownFace(t *testing.T) {
	regions := newRegionTable([]string{"A"})
	_, err := regions.BindSurface("bc", []string{"missing"}, fakeGroups(oneTetMesh()))
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Errorf("err = %v, want unknown-face failure naming the key", err)
	}
}

func TestRegionTableVolumeTagUnknownBody(t *testing.T) {
	regions := newRegionTable([]string{"A"})
	if _, err := regions.VolumeTag(9); err == nil {
		t.Error("unknown body index returned a tag")
	}
}
