// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"fmt"
	"strings"

	"oblikovati.org/api/wire"
	"oblikovati.org/getdp/getdp/femmodel"
)

// StudyBrowserPaneID is the add-in's browser pane holding the study tree.
const StudyBrowserPaneID = "com.oblikovati.getdp.tree"

// ShowStudyTree (re)declares the study browser pane from the current aggregate — the
// model surface of the UI (spec §4.2): every study with its Physics / Regions /
// Boundary Conditions / Mesh / Solver nodes. It snapshots the model under lock, then
// makes the host call OUTSIDE the lock.
func (e *Engine) ShowStudyTree() error {
	e.mu.Lock()
	nodes := studyNodes(e.analysis)
	e.mu.Unlock()
	_, err := e.api.Browser().SetPane(wire.BrowserPaneSpec{
		ID: StudyBrowserPaneID, Title: "GetDP Studies", Nodes: nodes,
	})
	return err
}

// refreshTree redraws the tree after a model mutation (best-effort: a redraw failure
// must not mask the mutation's own outcome).
func (e *Engine) refreshTree() { _ = e.ShowStudyTree() }

// studyNodes projects the aggregate into the tree — pure and directly testable.
//
//	nodes := studyNodes(femmodel.NewAnalysis())
//	// nodes[0].ID == "studies"
func studyNodes(a *femmodel.Analysis) []wire.BrowserNodeSpec {
	kids := make([]wire.BrowserNodeSpec, 0, len(a.Studies()))
	for _, s := range a.Studies() {
		kids = append(kids, studyNode(s, s == a.Active()))
	}
	return []wire.BrowserNodeSpec{{
		ID: "studies", Label: "Studies", IconSVG: iconSVG("newstudy"), Expanded: true,
		Menu:     []wire.BrowserMenuItem{{ID: "newstudy", Label: "New Study…"}},
		Children: kids,
	}}
}

// studyNode renders one study with its section nodes; the active study is suffixed.
func studyNode(s *femmodel.Study, active bool) wire.BrowserNodeSpec {
	label := s.Name()
	if active {
		label += "  [active]"
	}
	return wire.BrowserNodeSpec{
		ID: "study:" + s.ID(), Label: label, IconSVG: physicsIcon(s.Solver.Physics), Expanded: active,
		Menu: studyMenu(),
		Children: []wire.BrowserNodeSpec{
			{ID: "physics:" + s.ID(), Label: "Physics: " + string(s.Solver.Physics),
				IconSVG: iconSVG("studysettings"), Menu: editMenu()},
			regionsNode(s),
			constraintsNode(s),
			{ID: "mesh:" + s.ID(), Label: meshLabel(s.Mesh), IconSVG: iconSVG("meshsettings"), Menu: editMenu()},
			{ID: "solver:" + s.ID(), Label: "Solver", IconSVG: iconSVG("solversettings"), Menu: editMenu()},
		},
	}
}

// regionsNode lists the study's body regions, plus the surrounding air region for the EM
// physics that solve fields in the space around the part (so the air domain is a first-class,
// editable tree entry, not implicit).
func regionsNode(s *femmodel.Study) wire.BrowserNodeSpec {
	glyph := iconSVG("regions")
	kids := make([]wire.BrowserNodeSpec, 0, len(s.Regions())+1)
	for _, r := range s.Regions() {
		kids = append(kids, wire.BrowserNodeSpec{
			ID: "region:" + r.ID, Label: fmt.Sprintf("%s (%s)", r.Name, r.Material.Name),
			IconSVG: iconSVG("materials"), Menu: editMenu(),
		})
	}
	if femmodel.NeedsAir(s.Solver.Physics) {
		kids = append(kids, wire.BrowserNodeSpec{
			ID: "air:" + s.ID(), Label: airLabel(s.Solver.Air),
			IconSVG: iconSVG("airregion"), Menu: editMenu(),
		})
	}
	return wire.BrowserNodeSpec{ID: "regions:" + s.ID(), Label: "Regions", IconSVG: glyph,
		Expanded: true, Children: kids}
}

// airLabel summarizes the air region for its tree row.
func airLabel(air femmodel.AirRegion) string {
	switch air.Mode {
	case femmodel.AirAutomaticBox:
		if air.Truncation == femmodel.TruncationInfiniteShell {
			return "Air: automatic ∞-shell"
		}
		return fmt.Sprintf("Air: automatic box (pad %g×)", air.PaddingFactor)
	case femmodel.AirManualBodies:
		return "Air: manual bodies"
	default:
		return "Air: none"
	}
}

// constraintsNode lists the study's boundary conditions, flagging an empty set — a
// study without constraints cannot solve, so the tree shows the prerequisite.
func constraintsNode(s *femmodel.Study) wire.BrowserNodeSpec {
	label := "Boundary Conditions"
	if len(s.Constraints()) == 0 {
		label += "  [!]"
	}
	kids := make([]wire.BrowserNodeSpec, 0, len(s.Constraints()))
	for _, c := range s.Constraints() {
		kids = append(kids, wire.BrowserNodeSpec{
			ID: "bc:" + c.ID, Label: constraintLabel(c), IconSVG: constraintIcon(c.Kind),
			Menu: append(editMenu(), wire.BrowserMenuItem{ID: "delete", Label: "Delete"}),
		})
	}
	return wire.BrowserNodeSpec{ID: "bcs:" + s.ID(), Label: label,
		IconSVG: iconSVG("boundarycondition"), Expanded: true, Children: kids}
}

// constraintLabel summarizes one BC for its tree row, faces included so an unbound BC
// is visible at a glance.
func constraintLabel(c femmodel.ConstraintObject) string {
	suffix := fmt.Sprintf(" — %d face(s)", len(c.Faces))
	if len(c.Faces) == 0 {
		suffix = "  [!] no faces"
	}
	return c.Name + suffix
}

// meshLabel summarizes the mesh settings for the tree row.
func meshLabel(m femmodel.MeshObject) string {
	size := "auto"
	if m.SizeModelUnits > 0 {
		size = fmt.Sprintf("%g", m.SizeModelUnits)
	}
	order := "1st order"
	if m.SecondOrder {
		order = "2nd order"
	}
	return fmt.Sprintf("Mesh: %s, %s", size, order)
}

// physicsIcon / constraintIcon map model kinds onto bundled glyph keys.
func physicsIcon(p femmodel.PhysicsKind) string {
	switch p {
	case femmodel.PhysicsElectrokinetics:
		return iconSVG("elekin")
	case femmodel.PhysicsThermalTransient:
		return iconSVG("thermaltransient")
	default:
		return iconSVG("thermal")
	}
}

func constraintIcon(k femmodel.ConstraintKind) string {
	switch k {
	case femmodel.KindVoltage, femmodel.KindCurrent:
		return iconSVG("elekin")
	default:
		return iconSVG("thermal")
	}
}

func studyMenu() []wire.BrowserMenuItem {
	return []wire.BrowserMenuItem{
		{ID: "run", Label: "Run Study"},
		{ID: "setactive", Label: "Set Active"},
		{ID: "duplicate", Label: "Duplicate"},
		{ID: "delete", Label: "Delete"},
	}
}

func editMenu() []wire.BrowserMenuItem {
	return []wire.BrowserMenuItem{{ID: "edit", Label: "Edit…"}}
}

// handleStudyNode routes a tree gesture: context-menu actions and double-click edits.
// It runs OFF the session goroutine (panels/tree/status are host calls).
func (e *Engine) handleStudyNode(node, gesture, menuItem string) {
	action := menuItem
	if gesture == "doubleclick" || gesture == "open" {
		action = "edit"
	}
	switch action {
	case "run":
		e.launchStudy()
	case "newstudy":
		e.reportStatus("Pick a physics under the ribbon's New Study flyout.")
	case "setactive", "duplicate", "delete":
		e.mutateStudyFromNode(node, action)
	case "edit":
		e.openEditorForNode(node)
	}
}

// mutateStudyFromNode applies a study-level tree action and redraws.
func (e *Engine) mutateStudyFromNode(node, action string) {
	id, ok := strings.CutPrefix(node, "study:")
	if !ok {
		return
	}
	var err error
	e.withAnalysis(func(a *femmodel.Analysis) {
		switch action {
		case "setactive":
			err = a.SetActive(id)
		case "duplicate":
			_, err = a.DuplicateStudy(id)
		case "delete":
			err = a.RemoveStudy(id)
		}
	})
	if err != nil {
		e.reportStatus("GetDP: " + err.Error())
		return
	}
	e.refreshTree()
}

// openEditorForNode opens the task panel matching a tree node's Edit gesture.
func (e *Engine) openEditorForNode(node string) {
	kind, id, ok := strings.Cut(node, ":")
	if !ok {
		return
	}
	switch kind {
	case "physics", "study":
		e.openStudyPanel(id)
	case "mesh":
		e.openMeshPanel(id)
	case "solver":
		e.openSolverPanel(id)
	case "region":
		e.openRegionPanel(id)
	case "air":
		e.openAirPanel(id)
	case "bc":
		e.openConstraintPanel(id)
	}
}
