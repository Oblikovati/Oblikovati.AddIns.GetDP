// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"oblikovati.org/api/wire"
)

// boxHost is a fake host serving one box body and a two-face selection (an inlet face
// and an outlet face), enough to drive RunStudyOnHost end-to-end once the mesh/solve
// pipeline lands (M2+). Coordinates are in host model units (1 unit = 10 mm); the box
// is 20×1×1 model units, i.e. a 200×10×10 mm bar after the engine's cm→m scaling.
type boxHost struct {
	mu    sync.Mutex
	calls map[string]int
	box   [8][3]float64
}

func newBoxHost() *boxHost {
	const l, h = 20.0, 1.0
	return &boxHost{
		calls: map[string]int{},
		box: [8][3]float64{
			{0, 0, 0}, {l, 0, 0}, {l, h, 0}, {0, h, 0},
			{0, 0, h}, {l, 0, h}, {l, h, h}, {0, h, h},
		},
	}
}

const (
	inletFaceKey  = "inlet"
	outletFaceKey = "outlet"
)

func (b *boxHost) Call(method string, req []byte) ([]byte, error) {
	b.mu.Lock()
	b.calls[method]++
	b.mu.Unlock()
	switch method {
	case wire.MethodBodyList:
		// One solid body, index 0, with no assigned material (the study falls back to
		// the region default). A reference key is required so face binding can probe by body.
		return json.Marshal(wire.BodyListResult{Bodies: []wire.BodyInfo{
			{Index: 0, Name: "Solid1", Solid: true, Key: "body0"},
		}})
	case wire.MethodModelSelection:
		// The host encodes a selected face as "face/<url-base64 of the raw key>".
		refs := []string{encodeFaceRef(inletFaceKey), encodeFaceRef(outletFaceKey)}
		return json.Marshal(wire.SelectionResult{Count: 2, Refs: refs})
	case wire.MethodBodyCalculateFacets:
		return json.Marshal(b.bodyFacets())
	case wire.MethodFaceCalculateFacets:
		return b.faceFacets(req)
	default:
		return []byte("{}"), nil // graphics register/set return no body the engine reads
	}
}

func (b *boxHost) saw(method string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls[method] > 0
}

// bodyFacets returns the whole box surface as a raw triangle soup.
func (b *boxHost) bodyFacets() wire.FacetSetResult {
	quads := [6][4]int{{0, 3, 2, 1}, {4, 5, 6, 7}, {0, 1, 5, 4}, {1, 2, 6, 5}, {2, 3, 7, 6}, {3, 0, 4, 7}}
	var coords []float64
	var idx []int
	for _, q := range quads {
		coords, idx = appendQuad(coords, idx, b.box, q)
	}
	return wire.FacetSetResult{VertexCoordinates: coords, VertexIndices: idx}
}

// faceFacets returns the two triangles of the requested face (the x=0 face for the
// inlet key, the x=L face for the outlet key).
func (b *boxHost) faceFacets(req []byte) ([]byte, error) {
	var args wire.FaceFacetsArgs
	if err := json.Unmarshal(req, &args); err != nil {
		return nil, err
	}
	quad := [4]int{1, 2, 6, 5} // x=L (outlet)
	if args.FaceKey == inletFaceKey {
		quad = [4]int{0, 3, 7, 4} // x=0 (inlet)
	}
	var coords []float64
	var idx []int
	coords, idx = appendQuad(coords, idx, b.box, quad)
	return json.Marshal(wire.FacetSetResult{VertexCoordinates: coords, VertexIndices: idx})
}

// encodeFaceRef mirrors the host's selection encoding: "face/" + url-base64(raw key).
// The engine-side decoder (selection.go) lands with the M2 mesh pipeline.
func encodeFaceRef(rawKey string) string {
	return "face/" + base64.RawURLEncoding.EncodeToString([]byte(rawKey))
}

// appendQuad appends a quad's two triangles to the coordinate/index soup.
func appendQuad(coords []float64, idx []int, v [8][3]float64, q [4]int) ([]float64, []int) {
	base := len(coords) / 3
	for _, c := range q {
		coords = append(coords, v[c][0], v[c][1], v[c][2])
	}
	return coords, append(idx, base, base+1, base+2, base, base+2, base+3)
}

// TestBoxHostDrivesStubCommandEndToEnd fires the registered study command against the
// boxHost fixture and asserts the full M0 loop: Notify dispatch → coalesced study
// goroutine → pipeline (still the loud not-implemented stub) → status report. This is
// the harness the M2+ pipeline tests grow into (mesh → deck → solve → render).
func TestBoxHostDrivesStubCommandEndToEnd(t *testing.T) {
	b := newBoxHost()
	e := NewEngine(b)
	if err := e.Setup(); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	e.Notify(commandStartedEvent(RunStudyCommandID))
	waitIdle(e)
	if !b.saw(wire.MethodStatusSetText) {
		t.Fatal("study run never reported status through the boxHost")
	}
}

// TestBoxHostSelectionEncoding pins the face-ref encoding the M2 selection decoder must
// round-trip: "face/" + RawURLEncoding of the host's raw face key.
func TestBoxHostSelectionEncoding(t *testing.T) {
	resp, err := newBoxHost().Call(wire.MethodModelSelection, nil)
	if err != nil {
		t.Fatalf("selection: %v", err)
	}
	var sel wire.SelectionResult
	if err := json.Unmarshal(resp, &sel); err != nil {
		t.Fatalf("decode selection: %v", err)
	}
	if sel.Count != 2 || len(sel.Refs) != 2 {
		t.Fatalf("selection = %+v, want 2 face refs", sel)
	}
	for _, ref := range sel.Refs {
		if !strings.HasPrefix(ref, "face/") {
			t.Errorf("ref %q missing face/ prefix", ref)
		}
	}
}
