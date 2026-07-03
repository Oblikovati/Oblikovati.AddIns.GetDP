// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"fmt"
	"strconv"

	"oblikovati.org/getdp/getdp/femmodel"
)

// applyPanelEdit routes one control edit into the open draft (inline: no host call).
// Unparseable numbers are ignored — the draft keeps its last valid value, and OK
// commits only what parsed.
func (e *Engine) applyPanelEdit(windowID, controlID, value string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.panel == nil || e.panel.id != windowID {
		return
	}
	switch windowID {
	case tpStudyID, tpSolverID:
		e.panel.applyStudyEdit(controlID, value)
	case tpRegionID:
		e.panel.applyRegionEdit(controlID, value)
	case tpConstraintID:
		e.panel.applyConstraintEdit(controlID, value)
	case tpMeshID:
		e.panel.applyMeshEdit(controlID, value)
	}
}

// applyPanelReferences routes a referenceList change (the BC face picks) into the draft.
func (e *Engine) applyPanelReferences(windowID, controlID string, refs []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.panel == nil || e.panel.id != windowID || controlID != "faces" {
		return
	}
	e.panel.constraint.Faces = decodeSelectedFaces(refs)
}

// applyStudyEdit mutates the TP-1/TP-12 draft.
func (p *openPanel) applyStudyEdit(controlID, value string) {
	switch controlID {
	case "physics":
		p.physics = femmodel.PhysicsKind(value)
	case "tmax":
		setNum(&p.solver.TMax, value)
	case "dt":
		setNum(&p.solver.DT, value)
	case "theta":
		setNum(&p.solver.Theta, value)
	case "initial":
		setNum(&p.solver.Initial, value)
	}
}

// applyRegionEdit mutates the TP-2 draft.
func (p *openPanel) applyRegionEdit(controlID, value string) {
	switch controlID {
	case "name":
		p.region.Name = value
	case "material":
		p.region.Material.Name = value
	case "sigma":
		setNum(&p.region.Material.Sigma, value)
	case "k":
		setNum(&p.region.Material.K, value)
	case "rho":
		setNum(&p.region.Material.Rho, value)
	case "cp":
		setNum(&p.region.Material.Cp, value)
	}
}

// applyConstraintEdit mutates the TP-4/TP-6 draft.
func (p *openPanel) applyConstraintEdit(controlID, value string) {
	switch controlID {
	case "name":
		p.constraint.Name = value
	case "value":
		setNum(&p.constraint.Value, value)
	case "h":
		setNum(&p.constraint.H, value)
	case "tinf":
		setNum(&p.constraint.TInf, value)
	}
}

// applyMeshEdit mutates the TP-11 draft.
func (p *openPanel) applyMeshEdit(controlID, value string) {
	switch controlID {
	case "size":
		setNum(&p.mesh.SizeModelUnits, value)
	case "secondorder":
		p.mesh.SecondOrder = value == "true"
	}
}

// setNum parses a numeric edit into dst, keeping the previous value on bad input.
func setNum(dst *float64, value string) {
	if v, err := strconv.ParseFloat(value, 64); err == nil {
		*dst = v
	}
}

// closePanel commits (Accepted) or discards the open draft, then redraws the tree.
// Runs OFF the session goroutine (commit + redraw are host calls).
func (e *Engine) closePanel(windowID string, accepted bool) {
	e.mu.Lock()
	p := e.panel
	if p == nil || p.id != windowID {
		e.mu.Unlock()
		return
	}
	e.panel = nil
	var err error
	if accepted {
		err = commitPanel(e.analysis, p)
	}
	e.mu.Unlock()
	if err != nil {
		e.reportStatus("GetDP: " + err.Error())
		return
	}
	if accepted {
		e.refreshTree()
		e.refreshGlyphs()
	}
}

// commitPanel folds an accepted draft into the aggregate (caller holds the lock).
func commitPanel(a *femmodel.Analysis, p *openPanel) error {
	s, err := a.StudyByID(p.studyID)
	if err != nil {
		return err
	}
	switch p.id {
	case tpStudyID, tpSolverID:
		return commitStudyDraft(s, p)
	case tpRegionID:
		return s.UpdateRegion(p.region)
	case tpConstraintID:
		return commitConstraintDraft(s, p)
	case tpMeshID:
		s.Mesh = p.mesh
		return nil
	}
	return fmt.Errorf("unknown panel %q", p.id)
}

// commitStudyDraft applies TP-1/TP-12: a physics switch resets defaults and drops
// now-incompatible constraints (reported, not silent); time-grid edits apply directly.
func commitStudyDraft(s *femmodel.Study, p *openPanel) error {
	if p.id == tpStudyID && p.physics != s.Solver.Physics {
		dropped := s.SetPhysics(p.physics)
		if len(dropped) > 0 {
			return fmt.Errorf("physics switched to %s; %d incompatible boundary condition(s) removed", p.physics, len(dropped))
		}
		return nil
	}
	s.Solver = p.solver
	return nil
}

// commitConstraintDraft appends or updates the BC draft.
func commitConstraintDraft(s *femmodel.Study, p *openPanel) error {
	if p.isNew {
		_, err := s.AddConstraint(p.constraint)
		return err
	}
	return s.UpdateConstraint(p.constraint)
}
