// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"fmt"

	"oblikovati.org/api/wire"
	"oblikovati.org/getdp/getdp/demos"
	"oblikovati.org/getdp/getdp/femmodel"
)

// demoBuilder authors a demo's parametric geometry and returns its configured study.
type demoBuilder func(demos.Author) (demos.Study, error)

// demoEntry names a bundled demo's document and its builder.
type demoEntry struct {
	docName string
	build   demoBuilder
}

// demoRegistry maps each demo command to its document name and builder. Adding a physics
// demo is one line here plus a command/ribbon spot.
var demoRegistry = map[string]demoEntry{
	DemoBusbarCommandID:    {"GetDP Busbar Demo", demos.BuildBusbar},
	DemoHeatSinkCommandID:  {"GetDP Heat Sink Demo", demos.BuildHeatSink},
	DemoCapacitorCommandID: {"GetDP Capacitor Demo", demos.BuildCapacitor},
}

// buildDemo runs one bundled demo from its command: it builds the parametric part and study,
// refreshes the tree, and reports readiness. Solving is left to the Run command so the demo
// build stays fast and the two steps screenshot independently (issue #21).
func (e *Engine) buildDemo(id string) {
	name, err := e.RunDemo(id)
	if err != nil {
		e.reportStatus("GetDP demo failed: " + err.Error())
		return
	}
	e.refreshTree()
	e.reportStatus(fmt.Sprintf("GetDP: %q ready — Run Study to mesh and solve.", name))
}

// RunDemo builds a bundled demo's parametric geometry and configures its study synchronously,
// returning the study name. Exported so a live-capture driver (and future scripting) can run
// a demo without the command/event round-trip. It does not solve — call RunStudyOnHost next.
func (e *Engine) RunDemo(id string) (string, error) {
	entry, ok := demoRegistry[id]
	if !ok {
		return "", fmt.Errorf("unknown demo command %q", id)
	}
	return e.buildDemoOnHost(entry)
}

// buildDemoOnHost creates a fresh part document, replays the demo's geometry program over
// the host, and loads its study onto a new active study. It returns the study name.
func (e *Engine) buildDemoOnHost(entry demoEntry) (string, error) {
	doc, err := e.api.Documents().Create(wire.CreateDocumentArgs{Type: "part", Name: entry.docName})
	if err != nil {
		return "", fmt.Errorf("create demo document: %w", err)
	}
	if _, err := e.api.Documents().Activate(doc.ID); err != nil {
		return "", fmt.Errorf("activate demo document: %w", err)
	}
	study, err := entry.build(&clientAuthor{api: e.api})
	if err != nil {
		return "", err
	}
	return entry.docName, e.loadDemoStudy(entry.docName, study)
}

// loadDemoStudy adds a new active study of the demo's physics and loads its mesh size,
// dielectric/air overrides and constraints onto it. The default all-bodies region already
// carries the physics' default material (copper / aluminium / vacuum dielectric); a demo may
// override the permittivity (a real dielectric) and the air-box padding.
func (e *Engine) loadDemoStudy(name string, study demos.Study) error {
	var addErr error
	e.withAnalysis(func(a *femmodel.Analysis) {
		s := a.AddStudy(study.Physics)
		s.Rename(name)
		s.Mesh.SizeModelUnits = study.MeshModelUnits
		applyDemoMaterialAndAir(s, study)
		for _, c := range study.Constraints {
			if _, err := s.AddConstraint(c); err != nil {
				addErr = fmt.Errorf("load constraint %q: %w", c.Name, err)
				return
			}
		}
	})
	return addErr
}

// applyDemoMaterialAndAir folds a demo's dielectric permittivity onto the part region and its
// air-box padding onto the solver (both no-ops when the demo leaves them at 0).
func applyDemoMaterialAndAir(s *femmodel.Study, study demos.Study) {
	if study.Epsilon > 0 {
		if regs := s.Regions(); len(regs) > 0 {
			regs[0].Material.Epsilon = study.Epsilon
			_ = s.UpdateRegion(regs[0])
		}
	}
	if study.AirPadding > 0 {
		s.Solver.Air.PaddingFactor = study.AirPadding
	}
}
