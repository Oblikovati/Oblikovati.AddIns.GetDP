// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"encoding/json"
	"strings"

	"oblikovati.org/getdp/getdp/femmodel"
)

// onCommandStarted dispatches our registered commands. Everything that makes host
// calls (panels, tree redraws, meshing, solving) runs OFF the session goroutine.
func (e *Engine) onCommandStarted(ev []byte) {
	var c struct {
		Command string `json:"command"`
	}
	if json.Unmarshal(ev, &c) != nil || !strings.HasPrefix(c.Command, "GetDP.") {
		return
	}
	e.dispatchCommand(c.Command)
}

// dispatchCommand routes one GetDP command id.
func (e *Engine) dispatchCommand(id string) {
	switch id {
	case RunStudyCommandID:
		e.launchStudy()
	case StopSolveCommandID:
		e.stopStudy()
	case NewElectrokineticsCommandID:
		go e.addStudy(femmodel.PhysicsElectrokinetics)
	case NewThermalCommandID:
		go e.addStudy(femmodel.PhysicsThermalSteady)
	case NewThermalTransientCommandID:
		go e.addStudy(femmodel.PhysicsThermalTransient)
	case GenerateMeshCommandID:
		go e.meshOnly()
	case DemoBusbarCommandID, DemoHeatSinkCommandID:
		go e.buildDemo(id)
	default:
		e.dispatchEditorCommand(id)
	}
}

// dispatchEditorCommand routes the study mutators and flyout heads.
func (e *Engine) dispatchEditorCommand(id string) {
	switch id {
	case NewStudyCommandID:
		go e.reportStatus("New Study: pick a physics from the flyout (Electrokinetics / Thermal / Thermal Transient).")
	case AddBCCommandID:
		go e.reportStatus("Boundary Condition: pick a kind from the flyout, with the target faces selected.")
	case DuplicateStudyCommandID:
		go e.duplicateActive()
	case SetActiveStudyCommandID:
		go e.setActiveFromSelection()
	default:
		e.dispatchWindowCommand(id)
	}
}

// dispatchWindowCommand routes the panel and window openers.
func (e *Engine) dispatchWindowCommand(id string) {
	switch id {
	case StudySettingsCommandID:
		go e.openStudyPanel(e.activeStudyID())
	case EditRegionsCommandID, EditMaterialsCommandID:
		go e.openFirstRegionPanel()
	case MeshSettingsCommandID:
		go e.openMeshPanel(e.activeStudyID())
	case SolverSettingsCommandID:
		go e.openSolverPanel(e.activeStudyID())
	case ShowTreeCommandID:
		go func() { _ = e.ShowStudyTree() }()
	case ShowMonitorCommandID:
		go e.showMonitor("idle", nil)
	default:
		e.dispatchAddBC(id)
	}
}

// dispatchAddBC maps the BC flyout variants onto constraint kinds.
func (e *Engine) dispatchAddBC(id string) {
	kinds := map[string]femmodel.ConstraintKind{
		AddVoltageCommandID:     femmodel.KindVoltage,
		AddCurrentCommandID:     femmodel.KindCurrent,
		AddTemperatureCommandID: femmodel.KindTemperature,
		AddHeatFluxCommandID:    femmodel.KindHeatFlux,
		AddConvectionCommandID:  femmodel.KindConvection,
	}
	if kind, ok := kinds[id]; ok {
		go e.addConstraintFromSelection(kind)
	}
}

// activeStudyID snapshots the active study's id.
func (e *Engine) activeStudyID() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.analysis.Active().ID()
}

// addStudy appends a study of the given physics, redraws, and opens its TP-1.
func (e *Engine) addStudy(kind femmodel.PhysicsKind) {
	var id string
	e.withAnalysis(func(a *femmodel.Analysis) { id = a.AddStudy(kind).ID() })
	e.refreshTree()
	e.openStudyPanel(id)
}

// duplicateActive copies the active study.
func (e *Engine) duplicateActive() {
	var err error
	e.withAnalysis(func(a *femmodel.Analysis) { _, err = a.DuplicateStudy(a.Active().ID()) })
	if err != nil {
		e.reportStatus("GetDP: " + err.Error())
		return
	}
	e.refreshTree()
}

// setActiveFromSelection activates the tree-selected study (the ribbon path; the tree
// menu routes directly).
func (e *Engine) setActiveFromSelection() {
	e.mu.Lock()
	node := e.selectedNode
	e.mu.Unlock()
	id, ok := strings.CutPrefix(node, "study:")
	if !ok {
		e.reportStatus("Set Active: click a study in the GetDP tree first.")
		return
	}
	e.mutateStudyFromNode("study:"+id, "setactive")
}

// openFirstRegionPanel opens TP-2 for the active study's first region (the tree's
// per-region Edit reaches the others).
func (e *Engine) openFirstRegionPanel() {
	e.mu.Lock()
	regions := e.analysis.Active().Regions()
	e.mu.Unlock()
	if len(regions) == 0 {
		e.reportStatus("GetDP: the active study has no regions.")
		return
	}
	e.openRegionPanel(regions[0].ID)
}

// addConstraintFromSelection snapshots the current face selection into a new BC draft
// of the given kind and opens its editor. Kind×physics compatibility is checked up
// front so the panel never opens for an impossible combination.
func (e *Engine) addConstraintFromSelection(kind femmodel.ConstraintKind) {
	e.mu.Lock()
	active := e.analysis.Active()
	studyID, physics := active.ID(), active.Solver.Physics
	e.mu.Unlock()
	if err := checkKindAllowed(physics, kind); err != nil {
		e.reportStatus("GetDP: " + err.Error())
		return
	}
	sel, err := e.api.Model().Selection()
	if err != nil {
		e.reportStatus("GetDP: read selection: " + err.Error())
		return
	}
	c := femmodel.ConstraintObject{Kind: kind, Faces: decodeSelectedFaces(sel.Refs)}
	c.Name = defaultBCName(kind)
	e.showConstraintPanel(studyID, c, true)
}

// checkKindAllowed reproduces the femmodel gate for the pre-panel check.
func checkKindAllowed(p femmodel.PhysicsKind, k femmodel.ConstraintKind) error {
	probe := femmodel.ConstraintObject{Kind: k}
	s := femmodel.NewAnalysis().Active()
	dropped := s.SetPhysics(p)
	_ = dropped
	_, err := s.AddConstraint(probe)
	return err
}

// defaultBCName seeds the editor's name field per kind.
func defaultBCName(k femmodel.ConstraintKind) string { return string(k) }
