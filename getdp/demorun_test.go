// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"oblikovati.org/api/wire"
	"oblikovati.org/getdp/getdp/demos"
	"oblikovati.org/getdp/getdp/femmodel"
)

// demoHost is a fake host that answers just enough of the modelling API for a demo build to
// run without the app: it hands out unique sketch-point ids, always reports a healthy
// single-body extrude, and returns a face key encoding each probe point so the test can
// assert which face every boundary condition bound to.
type demoHost struct {
	mu       sync.Mutex
	calls    map[string]int
	nextPt   uint64
	extrudes int
}

func newDemoHost() *demoHost { return &demoHost{calls: map[string]int{}, nextPt: 1} }

func (h *demoHost) Call(method string, req []byte) ([]byte, error) {
	h.mu.Lock()
	h.calls[method]++
	h.mu.Unlock()
	switch method {
	case wire.MethodDocumentsCreate:
		return json.Marshal(wire.DocumentInfo{ID: 1, Name: "demo"})
	case wire.MethodSketchCreate:
		return json.Marshal(wire.CreateSketchResult{SketchIndex: h.calls[method] - 1, Plane: "XY"})
	case wire.MethodSketchAddEntity:
		return h.addEntity()
	case wire.MethodFeaturesAdd:
		return h.featuresAdd(req)
	case wire.MethodModelTree:
		return json.Marshal(wire.ModelTreeResult{Features: []wire.FeatureInfo{{ID: uint64(h.calls[method]), Name: "F"}}})
	case wire.MethodBodyList:
		return json.Marshal(wire.BodyListResult{Bodies: []wire.BodyInfo{{Index: 0, Solid: true, Key: "body0"}}})
	case wire.MethodBodyLocateUsingPoint:
		return h.locate(req)
	default:
		return []byte("{}"), nil // parameters.add, constraints, dimensions, activate, rename
	}
}

// addEntity returns a line entity with two fresh, unique point ids so the rectangle's welds
// and dimensions reference distinct points.
func (h *demoHost) addEntity() ([]byte, error) {
	h.mu.Lock()
	a, b := h.nextPt, h.nextPt+1
	h.nextPt += 2
	h.mu.Unlock()
	return json.Marshal(wire.AddSketchEntityResult{EntityID: a, Kind: "line", PointIDs: []uint64{a, b}})
}

// featuresAdd distinguishes an extrude (reports one healthy body) from a pattern (opaque OK).
func (h *demoHost) featuresAdd(req []byte) ([]byte, error) {
	var f struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(req, &f)
	if f.Kind == "extrude" {
		h.mu.Lock()
		h.extrudes++
		h.mu.Unlock()
		return json.Marshal(struct {
			Bodies  int  `json:"bodies"`
			Healthy bool `json:"healthy"`
		}{Bodies: 1, Healthy: true})
	}
	return []byte("{}"), nil
}

// locate returns a found face whose key encodes the probe point, so a bound BC reveals the
// face it landed on.
func (h *demoHost) locate(req []byte) ([]byte, error) {
	var args wire.LocateUsingPointArgs
	if err := json.Unmarshal(req, &args); err != nil {
		return nil, err
	}
	key := fmt.Sprintf("face@%.3f,%.3f,%.3f", args.Point[0], args.Point[1], args.Point[2])
	return json.Marshal(wire.LocateUsingPointResult{Found: true, Entity: wire.LocatedEntityInfo{Kind: "face", Key: key}})
}

func (h *demoHost) saw(method string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls[method]
}

// assertActiveStudy checks the engine's active study is named after the demo, runs the
// expected physics, and holds the expected constraint count.
func assertActiveStudy(t *testing.T, e *Engine, name string, physics femmodel.PhysicsKind, constraints int) {
	t.Helper()
	e.withAnalysis(func(a *femmodel.Analysis) {
		s := a.Active()
		if s.Name() != name {
			t.Errorf("active study = %q, want %q", s.Name(), name)
		}
		if s.Solver.Physics != physics {
			t.Errorf("active physics = %q, want %q", s.Solver.Physics, physics)
		}
		if len(s.Constraints()) != constraints {
			t.Errorf("active constraints = %d, want %d", len(s.Constraints()), constraints)
		}
	})
}

// activeConstraintKinds returns the active study's constraint kinds in order.
func activeConstraintKinds(e *Engine) []femmodel.ConstraintKind {
	var kinds []femmodel.ConstraintKind
	e.withAnalysis(func(a *femmodel.Analysis) {
		for _, c := range a.Active().Constraints() {
			kinds = append(kinds, c.Kind)
		}
	})
	return kinds
}

// activeConstraintFace returns the first face key of the i-th active-study constraint.
func activeConstraintFace(e *Engine, i int) string {
	var key string
	e.withAnalysis(func(a *femmodel.Analysis) {
		cs := a.Active().Constraints()
		if i < len(cs) && len(cs[i].Faces) > 0 {
			key = cs[i].Faces[0]
		}
	})
	return key
}

// TestBusbarDemoBuildsAndConfiguresStudy drives the busbar demo through the engine over the
// fake host and asserts the full host path: a document is created, the parametric geometry is
// authored (parameters, one rectangle sketch, one extrude), the two end faces are probed, and
// the active study is a two-electrode electrokinetics problem bound to those faces.
func TestBusbarDemoBuildsAndConfiguresStudy(t *testing.T) {
	h := newDemoHost()
	e := NewEngine(h)
	name, err := e.buildDemoOnHost(demoRegistry[DemoBusbarCommandID])
	if err != nil {
		t.Fatalf("buildDemoOnHost: %v", err)
	}
	if h.saw(wire.MethodDocumentsCreate) != 1 {
		t.Errorf("documents created = %d, want 1", h.saw(wire.MethodDocumentsCreate))
	}
	if want := len(demos.BusbarParams()); h.saw(wire.MethodParametersAdd) != want {
		t.Errorf("parameters published = %d, want %d", h.saw(wire.MethodParametersAdd), want)
	}
	if got := h.saw(wire.MethodSketchAddEntity); got != 4 {
		t.Errorf("sketch lines = %d, want 4 (one rectangle)", got)
	}
	if h.extrudes != 1 {
		t.Errorf("extrudes = %d, want 1", h.extrudes)
	}
	if got := h.saw(wire.MethodBodyLocateUsingPoint); got != 2 {
		t.Errorf("face probes = %d, want 2 end caps", got)
	}
	assertActiveStudy(t, e, name, femmodel.PhysicsElectrokinetics, 2)
	kinds := activeConstraintKinds(e)
	for _, k := range kinds {
		if k != femmodel.KindVoltage {
			t.Errorf("constraint kind %q, want voltage", k)
		}
	}
	if key := activeConstraintFace(e, 0); !strings.HasPrefix(key, "face@") {
		t.Errorf("V+ bound to %q, want a probed face key", key)
	}
}

// TestCapacitorDemoBuildsElectrostaticsStudy drives the capacitor demo through the engine over
// the fake host: a document, the parametric slab (2 params, one rectangle, one extrude), the
// two plate probes, and an electrostatics study whose part region took the dielectric εr and
// whose automatic air box took the demo's tight padding.
func TestCapacitorDemoBuildsElectrostaticsStudy(t *testing.T) {
	h := newDemoHost()
	e := NewEngine(h)
	name, err := e.buildDemoOnHost(demoRegistry[DemoCapacitorCommandID])
	if err != nil {
		t.Fatalf("buildDemoOnHost: %v", err)
	}
	if want := len(demos.CapacitorParams()); h.saw(wire.MethodParametersAdd) != want {
		t.Errorf("parameters published = %d, want %d", h.saw(wire.MethodParametersAdd), want)
	}
	if got := h.saw(wire.MethodSketchAddEntity); got != 4 {
		t.Errorf("sketch lines = %d, want 4 (one plate rectangle)", got)
	}
	if h.extrudes != 1 {
		t.Errorf("extrudes = %d, want 1 (dielectric slab)", h.extrudes)
	}
	if got := h.saw(wire.MethodBodyLocateUsingPoint); got != 2 {
		t.Errorf("face probes = %d, want 2 plates", got)
	}
	assertActiveStudy(t, e, name, femmodel.PhysicsElectrostatics, 2)
	e.withAnalysis(func(a *femmodel.Analysis) {
		s := a.Active()
		if regs := s.Regions(); len(regs) == 0 || regs[0].Material.Epsilon != 4 {
			t.Errorf("part region εr = %+v, want the demo dielectric 4", s.Regions())
		}
		if s.Solver.Air.Mode != femmodel.AirAutomaticBox || s.Solver.Air.PaddingFactor != 1.5 {
			t.Errorf("air = %+v, want automatic box padding 1.5", s.Solver.Air)
		}
	})
}

// TestHeatSinkDemoBuildsFinnedStudy drives the heat-sink demo and asserts its richer host
// path: a base plus a fin extrude, a linear fin pattern, and a steady-thermal study with a
// temperature BC plus one convection BC per fin.
func TestHeatSinkDemoBuildsFinnedStudy(t *testing.T) {
	h := newDemoHost()
	e := NewEngine(h)
	name, err := e.buildDemoOnHost(demoRegistry[DemoHeatSinkCommandID])
	if err != nil {
		t.Fatalf("buildDemoOnHost: %v", err)
	}
	if h.extrudes != 2 {
		t.Errorf("extrudes = %d, want base + fin", h.extrudes)
	}
	if got := h.saw(wire.MethodFeaturesRename); got != 2 {
		t.Errorf("feature renames = %d, want 2", got)
	}
	if got := h.saw(wire.MethodBodyLocateUsingPoint); got != 1+demos.HeatSinkFinCount {
		t.Errorf("face probes = %d, want base + %d fins", got, demos.HeatSinkFinCount)
	}
	assertActiveStudy(t, e, name, femmodel.PhysicsThermalSteady, 1+demos.HeatSinkFinCount)
	kinds := activeConstraintKinds(e)
	if kinds[0] != femmodel.KindTemperature {
		t.Errorf("first BC kind = %q, want temperature", kinds[0])
	}
	for _, k := range kinds[1:] {
		if k != femmodel.KindConvection {
			t.Errorf("fin BC kind = %q, want convection", k)
		}
	}
}
