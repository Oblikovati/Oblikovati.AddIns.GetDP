// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"fmt"
	"strconv"
	"strings"

	"oblikovati.org/api/client"
	"oblikovati.org/api/wire"
	"oblikovati.org/getdp/getdp/femmodel"
)

// Task-panel ids — one per editor family (spec §4.3): the windowId every
// value/reference event carries routes back to the open draft.
const (
	tpStudyID      = "com.oblikovati.getdp.tp.study"      // TP-1
	tpRegionID     = "com.oblikovati.getdp.tp.region"     // TP-2
	tpConstraintID = "com.oblikovati.getdp.tp.constraint" // TP-4/TP-6 (kind-specific rows)
	tpAirID        = "com.oblikovati.getdp.tp.air"        // TP-3
	tpMeshID       = "com.oblikovati.getdp.tp.mesh"       // TP-11
	tpSolverID     = "com.oblikovati.getdp.tp.solver"     // TP-12
)

// openPanel is the draft behind the open task panel: edits land here and commit to the
// aggregate only on OK — Cancel discards without touching the model.
type openPanel struct {
	id      string // which tp* panel is open
	studyID string
	isNew   bool // constraint editor: append on OK instead of update

	physics    femmodel.PhysicsKind
	solver     femmodel.SolverObject
	mesh       femmodel.MeshObject
	region     femmodel.RegionObject
	constraint femmodel.ConstraintObject
	air        femmodel.AirRegion
}

// showPanel stores the draft and shows its panel (host call OUTSIDE the lock).
func (e *Engine) showPanel(p *openPanel, spec wire.TaskPanelSpec) {
	e.mu.Lock()
	e.panel = p
	e.mu.Unlock()
	if _, err := e.api.TaskPanels().Show(spec); err != nil {
		e.reportStatus("GetDP: open panel: " + err.Error())
	}
}

// openStudyPanel opens TP-1 for a study: physics choice + transient grid.
func (e *Engine) openStudyPanel(studyID string) {
	s, err := e.studyByID(studyID)
	if err != nil {
		e.reportStatus("GetDP: " + err.Error())
		return
	}
	p := &openPanel{id: tpStudyID, studyID: studyID, physics: s.Solver.Physics, solver: s.Solver}
	e.showPanel(p, wire.TaskPanelSpec{
		ID: tpStudyID, Title: "Study Settings — " + s.Name(), Controls: studyControls(p),
	})
}

// studyControls renders TP-1 from the draft.
func studyControls(p *openPanel) []wire.PanelControlSpec {
	cs := []wire.PanelControlSpec{
		client.PanelDropdown("physics", "Physics", physicsOptions(), string(p.physics)),
	}
	if p.physics == femmodel.PhysicsThermalTransient {
		cs = append(cs,
			client.PanelValueEditor("tmax", "Total time (s)", formatNum(p.solver.TMax)),
			client.PanelValueEditor("dt", "Time step (s)", formatNum(p.solver.DT)),
			client.PanelValueEditor("theta", "Theta (1=implicit Euler)", formatNum(p.solver.Theta)),
			client.PanelValueEditor("initial", "Initial temperature (K)", formatNum(p.solver.Initial)),
		)
	}
	return cs
}

func physicsOptions() []string {
	return []string{string(femmodel.PhysicsElectrokinetics),
		string(femmodel.PhysicsThermalSteady), string(femmodel.PhysicsThermalTransient)}
}

// openRegionPanel opens TP-2 for a region: name + material properties.
func (e *Engine) openRegionPanel(regionID string) {
	studyID, _, _ := strings.Cut(regionID, "/")
	s, err := e.studyByID(studyID)
	if err != nil {
		e.reportStatus("GetDP: " + err.Error())
		return
	}
	for _, r := range s.Regions() {
		if r.ID == regionID {
			p := &openPanel{id: tpRegionID, studyID: studyID, region: r}
			e.showPanel(p, wire.TaskPanelSpec{
				ID: tpRegionID, Title: "Region — " + r.Name, Controls: regionControls(r)})
			return
		}
	}
	e.reportStatus("GetDP: no region " + regionID)
}

// regionControls renders TP-2 from the draft.
func regionControls(r femmodel.RegionObject) []wire.PanelControlSpec {
	return []wire.PanelControlSpec{
		client.PanelTextBox("name", "Name", r.Name),
		client.PanelTextBox("material", "Material name", r.Material.Name),
		client.PanelValueEditor("sigma", "Conductivity σ (S/m)", formatNum(r.Material.Sigma)),
		client.PanelValueEditor("k", "Thermal conductivity k (W/m·K)", formatNum(r.Material.K)),
		client.PanelValueEditor("rho", "Density ρ (kg/m³)", formatNum(r.Material.Rho)),
		client.PanelValueEditor("cp", "Specific heat c (J/kg·K)", formatNum(r.Material.Cp)),
	}
}

// openConstraintPanel opens the BC editor for an existing constraint.
func (e *Engine) openConstraintPanel(constraintID string) {
	studyID, _, _ := strings.Cut(constraintID, "/")
	s, err := e.studyByID(studyID)
	if err != nil {
		e.reportStatus("GetDP: " + err.Error())
		return
	}
	for _, c := range s.Constraints() {
		if c.ID == constraintID {
			e.showConstraintPanel(studyID, c, false)
			return
		}
	}
	e.reportStatus("GetDP: no boundary condition " + constraintID)
}

// showConstraintPanel opens TP-4/TP-6 over a constraint draft (new or existing).
func (e *Engine) showConstraintPanel(studyID string, c femmodel.ConstraintObject, isNew bool) {
	p := &openPanel{id: tpConstraintID, studyID: studyID, constraint: c, isNew: isNew}
	e.showPanel(p, wire.TaskPanelSpec{
		ID: tpConstraintID, Title: fmt.Sprintf("Boundary Condition — %s", c.Kind),
		Controls: constraintControls(c),
	})
}

// constraintControls renders the kind-specific BC rows: shared name+faces, then the
// kind's value fields (spec §4.3 — the family editor re-renders per kind).
func constraintControls(c femmodel.ConstraintObject) []wire.PanelControlSpec {
	cs := []wire.PanelControlSpec{
		client.PanelTextBox("name", "Name", c.Name),
		client.PanelReferenceList("faces", "Faces", []string{"face"}, faceRows(c.Faces)),
	}
	switch c.Kind {
	case femmodel.KindVoltage:
		cs = append(cs, client.PanelValueEditor("value", "Potential (V)", formatNum(c.Value)))
	case femmodel.KindCurrent:
		cs = append(cs, client.PanelValueEditor("value", "Total current (A)", formatNum(c.Value)))
	case femmodel.KindTemperature:
		cs = append(cs, client.PanelValueEditor("value", "Temperature (K)", formatNum(c.Value)))
	case femmodel.KindHeatFlux:
		cs = append(cs, client.PanelValueEditor("value", "Total heat rate (W)", formatNum(c.Value)))
	case femmodel.KindConvection:
		cs = append(cs,
			client.PanelValueEditor("h", "Film coefficient h (W/m²·K)", formatNum(c.H)),
			client.PanelValueEditor("tinf", "Ambient T∞ (K)", formatNum(c.TInf)))
	}
	return cs
}

// faceRows renders stored raw face keys back into reference rows.
func faceRows(keys []string) []wire.PanelReferenceRow {
	rows := make([]wire.PanelReferenceRow, len(keys))
	for i, k := range keys {
		rows[i] = wire.PanelReferenceRow{Ref: encodeFaceRef(k), Label: fmt.Sprintf("Face %d", i+1)}
	}
	return rows
}

// openAirPanel opens TP-3 for a study: the surrounding air domain the field solves in.
func (e *Engine) openAirPanel(studyID string) {
	s, err := e.studyByID(studyID)
	if err != nil {
		e.reportStatus("GetDP: " + err.Error())
		return
	}
	p := &openPanel{id: tpAirID, studyID: studyID, air: s.Solver.Air}
	e.showPanel(p, wire.TaskPanelSpec{
		ID: tpAirID, Title: "Air Region — " + s.Name(), Controls: airControls(p),
	})
}

// reshowAirPanel re-renders TP-3 from the current draft — used when a mode or truncation
// toggle changes which controls apply (spec §4.3: the family editor re-renders in place).
// Runs off the session goroutine because Show is a host call.
func (e *Engine) reshowAirPanel() {
	e.mu.Lock()
	p := e.panel
	if p == nil || p.id != tpAirID {
		e.mu.Unlock()
		return
	}
	spec := wire.TaskPanelSpec{ID: tpAirID, Title: "Air Region", Controls: airControls(p)}
	e.mu.Unlock()
	if _, err := e.api.TaskPanels().Show(spec); err != nil {
		e.reportStatus("GetDP: air panel: " + err.Error())
	}
}

// airControls renders TP-3 from the draft: a mode dropdown, then the mode-specific controls
// (an automatic box exposes its padding and far-boundary truncation; None and Manual show a
// hint). The trailing hint states what the chosen mode does so the domain is never implicit.
func airControls(p *openPanel) []wire.PanelControlSpec {
	cs := []wire.PanelControlSpec{
		client.PanelDropdown("mode", "Air region", airModeOptions(), airModeLabel(p.air.Mode)),
	}
	switch p.air.Mode {
	case femmodel.AirAutomaticBox:
		return append(cs, autoBoxControls(p.air)...)
	case femmodel.AirManualBodies:
		return append(cs, client.PanelLabel("hint",
			"Manual air bodies activate with multi-body air support; use Automatic box for a single-part study."))
	default: // AirNone
		return append(cs, client.PanelLabel("hint",
			"No air region — the field is confined to the part (conduction/thermal, or an electrostatics problem with every boundary fixed)."))
	}
}

// autoBoxControls are the automatic-padded-box rows: the padding slider, the infinite-shell
// toggle, and (when the shell is on) its two radii.
func autoBoxControls(air femmodel.AirRegion) []wire.PanelControlSpec {
	shell := air.Truncation == femmodel.TruncationInfiniteShell
	cs := []wire.PanelControlSpec{
		client.PanelSlider("padding", "Box padding (× part size)", air.PaddingFactor, 1, 10, 0.5),
		client.PanelCheckBox("infshell", "Infinite-shell truncation (open boundary)", shell),
	}
	if shell {
		cs = append(cs,
			client.PanelValueEditor("rint", "Shell inner radius (model units)", formatNum(air.ShellRint)),
			client.PanelValueEditor("rext", "Shell outer radius (model units)", formatNum(air.ShellRext)))
	}
	return append(cs, client.PanelLabel("hint",
		"A padded air box is generated around the part; the field solves in the part and the surrounding air."))
}

// airModeOptions / airModeLabel / parseAirMode map the AirMode enum to the dropdown's human
// labels (the label is what the host round-trips as the control value).
func airModeOptions() []string { return []string{"Automatic box", "Manual bodies", "None"} }

func airModeLabel(m femmodel.AirMode) string {
	switch m {
	case femmodel.AirAutomaticBox:
		return "Automatic box"
	case femmodel.AirManualBodies:
		return "Manual bodies"
	default:
		return "None"
	}
}

func parseAirMode(label string) femmodel.AirMode {
	switch label {
	case "Automatic box":
		return femmodel.AirAutomaticBox
	case "Manual bodies":
		return femmodel.AirManualBodies
	default:
		return femmodel.AirNone
	}
}

// openMeshPanel opens TP-11 for a study.
func (e *Engine) openMeshPanel(studyID string) {
	s, err := e.studyByID(studyID)
	if err != nil {
		e.reportStatus("GetDP: " + err.Error())
		return
	}
	p := &openPanel{id: tpMeshID, studyID: studyID, mesh: s.Mesh}
	e.showPanel(p, wire.TaskPanelSpec{
		ID: tpMeshID, Title: "Mesh Settings — " + s.Name(),
		Controls: []wire.PanelControlSpec{
			client.PanelValueEditor("size", "Max element size (model units, 0=auto)", formatNum(s.Mesh.SizeModelUnits)),
			client.PanelCheckBox("secondorder", "Second-order tetrahedra", s.Mesh.SecondOrder),
		},
	})
}

// openSolverPanel opens TP-12 for a study (M3: the transient grid; static studies show
// the linear-solver note until the SPARSKIT knobs ship with M5 nonlinear work).
func (e *Engine) openSolverPanel(studyID string) {
	s, err := e.studyByID(studyID)
	if err != nil {
		e.reportStatus("GetDP: " + err.Error())
		return
	}
	p := &openPanel{id: tpSolverID, studyID: studyID, solver: s.Solver}
	e.showPanel(p, wire.TaskPanelSpec{
		ID: tpSolverID, Title: "Solver Settings — " + s.Name(), Controls: solverControls(p),
	})
}

// solverControls renders TP-12 from the draft.
func solverControls(p *openPanel) []wire.PanelControlSpec {
	if p.solver.Physics != femmodel.PhysicsThermalTransient {
		return []wire.PanelControlSpec{client.PanelLabel("note",
			"Static linear study — solved with the vendored SPARSKIT direct/iterative solver.")}
	}
	return []wire.PanelControlSpec{
		client.PanelValueEditor("tmax", "Total time (s)", formatNum(p.solver.TMax)),
		client.PanelValueEditor("dt", "Time step (s)", formatNum(p.solver.DT)),
		client.PanelValueEditor("theta", "Theta (1=implicit Euler)", formatNum(p.solver.Theta)),
		client.PanelValueEditor("initial", "Initial temperature (K)", formatNum(p.solver.Initial)),
	}
}

// studyByID snapshots a study pointer under lock.
func (e *Engine) studyByID(id string) (*femmodel.Study, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.analysis.StudyByID(id)
}

// formatNum renders a float without trailing noise.
func formatNum(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
