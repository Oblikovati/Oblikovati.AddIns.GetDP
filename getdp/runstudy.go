// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"oblikovati.org/api/wire"
	"oblikovati.org/getdp/getdp/femmodel"
)

// StudyResult is what one solved study reports back to the UI surfaces.
type StudyResult struct {
	Physics    femmodel.PhysicsKind
	Elements   int
	FieldLabel string
	FieldUnit  string
	FieldMin   float64
	FieldMax   float64
	Scalars    []ScalarResult
}

// ScalarResult is one solved global quantity (a Format Table output).
type ScalarResult struct {
	Label string
	Unit  string
	Value float64
}

// Summary returns the one-line outcome for the host status bar.
func (r *StudyResult) Summary() string {
	msg := fmt.Sprintf("GetDP %s: %d elements, %s %.4g…%.4g %s",
		r.Physics, r.Elements, r.FieldLabel, r.FieldMin, r.FieldMax, r.FieldUnit)
	for _, s := range r.Scalars {
		msg += fmt.Sprintf(", %s %.4g %s", s.Label, s.Value, s.Unit)
	}
	return msg
}

// runInputs snapshots the active study under lock — the pipeline never touches the
// live aggregate.
type runInputs struct {
	physics     femmodel.PhysicsKind
	solver      femmodel.SolverObject
	mesh        femmodel.MeshObject
	air         femmodel.AirRegion
	regions     []femmodel.RegionObject
	constraints []femmodel.ConstraintObject
}

// snapshotRun copies the active study's run-relevant state.
func (e *Engine) snapshotRun() runInputs {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.analysis.Active()
	return runInputs{
		physics: s.Solver.Physics, solver: s.Solver, mesh: s.Mesh, air: s.Solver.Air,
		regions:     append([]femmodel.RegionObject(nil), s.Regions()...),
		constraints: append([]femmodel.ConstraintObject(nil), s.Constraints()...),
	}
}

// needsAir reports whether this run meshes a surrounding air region.
func (in runInputs) needsAir() bool {
	return femmodel.NeedsAir(in.physics) && in.air.Mode == femmodel.AirAutomaticBox
}

// RunStudyOnHost runs the active study end-to-end against the live host: mesh →
// physical groups → generated .pro → GetDP solve → .pos/table parse → flood plot.
func (e *Engine) RunStudyOnHost(ctx context.Context) (*StudyResult, error) {
	in := e.snapshotRun()
	staged, err := e.stageStudy(ctx, in)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(staged.dir)
	e.showMonitor("solving with GetDP…", nil)
	log, err := runGetDP(ctx, staged.bins.getdp, getdpRun{
		ProPath: "study.pro", MshPath: "study.msh",
		Resolution: staged.outs.Resolution, PostOps: staged.outs.PostOps, Dir: staged.dir,
	})
	e.showMonitor("post-processing…", tailLines(log, 12))
	if err != nil {
		return nil, err
	}
	return e.collectResults(in, staged)
}

// stagedStudy carries the pipeline artifacts between the stage and collect halves.
type stagedStudy struct {
	bins solverBinaries
	dir  string
	mesh *TetMesh
	outs DeckOutputs
}

// stageStudy runs the pre-solve pipeline: solvers, meshing, binding, deck generation,
// and file staging.
func (e *Engine) stageStudy(ctx context.Context, in runInputs) (*stagedStudy, error) {
	bins, err := findSolverBinaries()
	if err != nil {
		return nil, err
	}
	e.showMonitor("meshing…", nil)
	dir, mesh, solids, err := e.meshStudy(ctx, bins, in)
	if err != nil {
		return nil, err
	}
	deckText, outs, regions, err := e.buildStudyDeck(in, mesh, solids)
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	if err := stageFiles(dir, deckText, mesh, regions); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	return &stagedStudy{bins: bins, dir: dir, mesh: mesh, outs: outs}, nil
}

// meshStudy discovers bodies and volume-meshes them in a fresh workdir.
func (e *Engine) meshStudy(ctx context.Context, bins solverBinaries, in runInputs) (string, *TetMesh, []wire.BodyInfo, error) {
	solids, err := e.solidBodies()
	if err != nil {
		return "", nil, nil, err
	}
	dir, err := os.MkdirTemp("", "getdp-study-*")
	if err != nil {
		return "", nil, nil, fmt.Errorf("create study workdir: %w", err)
	}
	opts := MeshOptions{Size: in.mesh.SizeModelUnits, Order: FirstOrderTet}
	if in.mesh.SecondOrder {
		opts.Order = SecondOrderTet
	}
	mesh, err := e.meshForStudy(ctx, bins, opts, solids, in, dir)
	if err != nil {
		os.RemoveAll(dir)
		return "", nil, nil, err
	}
	return dir, mesh, solids, nil
}

// meshForStudy picks the mesher: a conformal part+air run for air-needing studies (single
// part body), the per-body solid mesher otherwise.
func (e *Engine) meshForStudy(ctx context.Context, bins solverBinaries, opts MeshOptions, solids []wire.BodyInfo, in runInputs, dir string) (*TetMesh, error) {
	if !in.needsAir() {
		return e.meshSolidBodies(ctx, bins, opts, solids, dir)
	}
	if len(solids) != 1 {
		return nil, fmt.Errorf("automatic air region supports a single part body (found %d); "+
			"model the extra bodies into one, or assign a body the Air role", len(solids))
	}
	surface, err := e.pullSurface(solids[0].Index)
	if err != nil {
		return nil, err
	}
	spec := AirSpec{PaddingFactor: in.air.PaddingFactor}
	return NewGmshMesher(bins.gmsh).MeshWithAir(ctx, surface, spec, opts, dir)
}

// buildStudyDeck binds faces, resolves specs, and generates the deck; the returned
// region table is what stageFiles feeds the MSH writer (both sides share its tags).
func (e *Engine) buildStudyDeck(in runInputs, mesh *TetMesh, solids []wire.BodyInfo) (string, DeckOutputs, *RegionTable, error) {
	groups, err := e.buildFaceGroups(constraintFaceKeys(in.constraints), mesh, solids)
	if err != nil {
		return "", DeckOutputs{}, nil, err
	}
	regions := newRegionTable(bodyNames(solids))
	if in.needsAir() {
		regions.addAirVolume()
	}
	rc := &ResolveContext{Model: &SolveModel{}, Mesh: mesh, Groups: groups, Regions: regions}
	if err := resolveSpecs(specsFrom(in.constraints), rc); err != nil {
		return "", DeckOutputs{}, nil, err
	}
	if in.needsAir() {
		if err := bindFarField(rc, mesh); err != nil {
			return "", DeckOutputs{}, nil, err
		}
	}
	writer, err := WriterFor(PhysicsKind(in.physics))
	if err != nil {
		return "", DeckOutputs{}, nil, err
	}
	deck, outs, err := writer.BuildDeck(DeckInput{
		Regions: regions, Model: rc.Model, Materials: materialsByTag(in, regions, mesh),
		Order: meshOrderOf(in.mesh), Transient: transientOf(in),
	})
	if err != nil {
		return "", DeckOutputs{}, nil, err
	}
	return deck.Render(), outs, regions, nil
}

// stageFiles writes study.pro and study.msh into the workdir.
func stageFiles(dir, deckText string, mesh *TetMesh, regions *RegionTable) error {
	if err := os.WriteFile(filepath.Join(dir, "study.pro"), []byte(deckText), 0o644); err != nil {
		return fmt.Errorf("write study.pro: %w", err)
	}
	return writeFile(filepath.Join(dir, "study.msh"), func(f *os.File) error {
		return writeMSH(f, mesh, regions)
	})
}

// collectResults parses the field + table outputs and renders the flood plot.
func (e *Engine) collectResults(in runInputs, staged *stagedStudy) (*StudyResult, error) {
	res := &StudyResult{Physics: in.physics, Elements: len(staged.mesh.Elements)}
	if len(staged.outs.Fields) > 0 {
		field := staged.outs.Fields[0]
		nodal, lo, hi, err := parsePosNodalField(filepath.Join(staged.dir, field.Path), staged.mesh)
		if err != nil {
			return nil, err
		}
		if err := e.renderScalarField(staged.mesh, nodal, lo, hi, in.needsAir()); err != nil {
			return nil, err
		}
		res.FieldLabel, res.FieldUnit, res.FieldMin, res.FieldMax = field.Label, field.Unit, lo, hi
	}
	for _, tbl := range staged.outs.Tables {
		if v, err := readLastTableValue(filepath.Join(staged.dir, tbl.Path)); err == nil {
			res.Scalars = append(res.Scalars, ScalarResult{Label: tbl.Label, Unit: tbl.Unit, Value: v})
		}
	}
	e.showMonitor("done: "+res.Summary(), nil)
	return res, nil
}

// meshOnly runs just the meshing half (the Generate Mesh command) and reports counts.
func (e *Engine) meshOnly() {
	in := e.snapshotRun()
	bins, err := findSolverBinaries()
	if err != nil {
		e.reportStatus("GetDP: " + err.Error())
		return
	}
	e.showMonitor("meshing…", nil)
	dir, mesh, _, err := e.meshStudy(context.Background(), bins, in)
	if err != nil {
		e.reportStatus("GetDP mesh failed: " + err.Error())
		return
	}
	defer os.RemoveAll(dir)
	msg := fmt.Sprintf("GetDP mesh: %d nodes, %d tetrahedra", len(mesh.Nodes), len(mesh.Elements))
	e.showMonitor(msg, nil)
	e.reportStatus(msg)
}

// bindFarField binds the air box's outer boundary and pins it to zero potential — the
// far-field reference the electrostatic field decays toward.
func bindFarField(rc *ResolveContext, mesh *TetMesh) error {
	tag, err := rc.Regions.BindOuterBoundary(mesh)
	if err != nil {
		return err
	}
	rc.Model.BoundPotentials = append(rc.Model.BoundPotentials, BoundPotential{
		Kind: KindVoltage, RegionTag: tag, Name: "FarField", Value: 0,
	})
	return nil
}

// constraintFaceKeys unions every constraint's face keys.
func constraintFaceKeys(cs []femmodel.ConstraintObject) []string {
	var keys []string
	for _, c := range cs {
		keys = append(keys, c.Faces...)
	}
	return keys
}

// bodyNames projects solids into region-table display names.
func bodyNames(solids []wire.BodyInfo) []string {
	names := make([]string, len(solids))
	for i, b := range solids {
		names[i] = b.Name
	}
	return names
}

// specsFrom converts constraint intents into resolvable specs. The femmodel and engine
// kind strings are value-identical by construction.
func specsFrom(cs []femmodel.ConstraintObject) []ConstraintSpec {
	var specs []ConstraintSpec
	for _, c := range cs {
		switch c.Kind {
		case femmodel.KindVoltage, femmodel.KindTemperature:
			specs = append(specs, DirichletSpec{SpecKind: ConstraintKind(c.Kind),
				SpecName: c.Name, FaceKeys: c.Faces, Value: c.Value})
		default:
			specs = append(specs, FluxSpec{SpecKind: ConstraintKind(c.Kind),
				SpecName: c.Name, FaceKeys: c.Faces, Value: c.Value, H: c.H, TInf: c.TInf})
		}
	}
	return specs
}

// materialsByTag maps each physical volume tag to its region's material: a body takes
// the material of the region listing it, else the material of the catch-all region
// (nil Bodies). The generated air volume is always vacuum (εr = 1) regardless of the
// part's dielectric, so it is never resolved through the part's region list.
func materialsByTag(in runInputs, regions *RegionTable, _ *TetMesh) map[int]Material {
	out := make(map[int]Material, len(regions.Volumes))
	for _, v := range regions.Volumes {
		if v.Body == airBodyIndex {
			out[v.Tag] = Material{Epsilon: 1}
			continue
		}
		out[v.Tag] = materialForBody(in.regions, v.Body)
	}
	return out
}

// materialForBody resolves one body's material through the region list.
func materialForBody(regions []femmodel.RegionObject, body int) Material {
	var catchAll *femmodel.MaterialProps
	for i, r := range regions {
		if r.Bodies == nil {
			catchAll = &regions[i].Material
			continue
		}
		if containsBody(r.Bodies, body) {
			return toMaterial(r.Material)
		}
	}
	if catchAll != nil {
		return toMaterial(*catchAll)
	}
	return Material{}
}

func containsBody(list []int, b int) bool {
	for _, x := range list {
		if x == b {
			return true
		}
	}
	return false
}

func toMaterial(m femmodel.MaterialProps) Material {
	return Material{Sigma: m.Sigma, K: m.K, Rho: m.Rho, Cp: m.Cp, Epsilon: m.Epsilon}
}

// meshOrderOf maps mesh settings onto the integration-rule order.
func meshOrderOf(m femmodel.MeshObject) int {
	if m.SecondOrder {
		return 2
	}
	return 1
}

// transientOf builds the theta grid for transient studies (nil otherwise).
func transientOf(in runInputs) *TransientSpec {
	if in.physics != femmodel.PhysicsThermalTransient {
		return nil
	}
	return &TransientSpec{TMax: in.solver.TMax, DT: in.solver.DT,
		Theta: in.solver.Theta, Initial: in.solver.Initial}
}
