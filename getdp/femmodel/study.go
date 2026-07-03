// SPDX-License-Identifier: GPL-2.0-only

package femmodel

import "fmt"

// PhysicsKind mirrors the engine's physics registry (kept as its own string type so
// this package stays dependency-free; the engine converts by value).
type PhysicsKind string

// Shipped physics (M3+). Later milestones append here AND in the engine registry.
const (
	PhysicsElectrokinetics  PhysicsKind = "electrokinetics"
	PhysicsThermalSteady    PhysicsKind = "thermal"
	PhysicsThermalTransient PhysicsKind = "thermal transient"
	PhysicsElectrostatics   PhysicsKind = "electrostatics"
	PhysicsMagnetostatics   PhysicsKind = "magnetostatics"
)

// Study is one simulation study: physics + solver settings, mesh settings, body
// regions with materials, and constraint intents. It is what the tree shows under one
// study node and what the runner projects into the pipeline.
type Study struct {
	id   string
	name string

	Solver      SolverObject
	Mesh        MeshObject
	regions     []RegionObject
	constraints []ConstraintObject
	coils       []CoilObject

	nextRegion     int
	nextConstraint int
	nextCoil       int
}

// newStudy builds a study with the physics' defaults and one all-bodies region.
func newStudy(id, name string, kind PhysicsKind) *Study {
	s := &Study{id: id, name: name, Solver: defaultSolver(kind), Mesh: defaultMesh()}
	s.AddRegion("All bodies", nil) // nil bodies = every solid body (runner semantics)
	return s
}

// ID returns the study's stable id (unique within the Analysis).
func (s *Study) ID() string { return s.id }

// Name returns the display name shown in the browser tree.
func (s *Study) Name() string { return s.name }

// Rename sets the display name.
func (s *Study) Rename(name string) { s.name = name }

// Regions returns the body regions in creation order.
func (s *Study) Regions() []RegionObject { return s.regions }

// Constraints returns the constraint intents in creation order.
func (s *Study) Constraints() []ConstraintObject { return s.constraints }

// Coils returns the current-source coils in creation order (magnetostatics).
func (s *Study) Coils() []CoilObject { return s.coils }

// AddCoil appends a current-source coil and returns its id.
func (s *Study) AddCoil(c CoilObject) string {
	s.nextCoil++
	c.ID = fmt.Sprintf("%s/coil%d", s.id, s.nextCoil)
	if c.Name == "" {
		c.Name = fmt.Sprintf("Coil %d", s.nextCoil)
	}
	s.coils = append(s.coils, c)
	return c.ID
}

// RemoveCoil deletes a coil by id.
func (s *Study) RemoveCoil(id string) error {
	for i := range s.coils {
		if s.coils[i].ID == id {
			s.coils = append(s.coils[:i], s.coils[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("no coil with id %q in study %q", id, s.name)
}

// AddRegion appends a region (defaults to the physics' default material) and returns
// its id. bodies lists merged-mesh body indexes; nil means "all bodies".
func (s *Study) AddRegion(name string, bodies []int) string {
	s.nextRegion++
	id := fmt.Sprintf("%s/region%d", s.id, s.nextRegion)
	s.regions = append(s.regions, RegionObject{
		ID: id, Name: name, Bodies: bodies, Material: defaultMaterial(s.Solver.Physics),
	})
	return id
}

// UpdateRegion replaces the region with r.ID. Assigning a body already owned by
// another region is rejected: every body belongs to at most one region.
func (s *Study) UpdateRegion(r RegionObject) error {
	for _, other := range s.regions {
		if other.ID == r.ID {
			continue
		}
		for _, b := range r.Bodies {
			if containsInt(other.Bodies, b) {
				return fmt.Errorf("body %d already belongs to region %q — a body has exactly one region", b, other.Name)
			}
		}
	}
	for i := range s.regions {
		if s.regions[i].ID == r.ID {
			s.regions[i] = r
			return nil
		}
	}
	return fmt.Errorf("no region with id %q in study %q", r.ID, s.name)
}

// RemoveRegion deletes a region by id (the last region cannot be removed: a study
// always scopes at least one region).
func (s *Study) RemoveRegion(id string) error {
	if len(s.regions) == 1 {
		return fmt.Errorf("cannot remove region %q: a study keeps at least one region", id)
	}
	for i := range s.regions {
		if s.regions[i].ID == id {
			s.regions = append(s.regions[:i], s.regions[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("no region with id %q in study %q", id, s.name)
}

// AddConstraint appends a constraint intent after validating kind×physics
// compatibility, returning its id.
func (s *Study) AddConstraint(c ConstraintObject) (string, error) {
	if !kindAllowed(s.Solver.Physics, c.Kind) {
		return "", fmt.Errorf("constraint kind %q does not apply to %s studies (allowed: %v)",
			c.Kind, s.Solver.Physics, allowedKinds(s.Solver.Physics))
	}
	s.nextConstraint++
	c.ID = fmt.Sprintf("%s/bc%d", s.id, s.nextConstraint)
	if c.Name == "" {
		c.Name = fmt.Sprintf("%s %d", c.Kind, s.nextConstraint)
	}
	s.constraints = append(s.constraints, c)
	return c.ID, nil
}

// UpdateConstraint replaces the constraint with c.ID (kind compatibility re-checked).
func (s *Study) UpdateConstraint(c ConstraintObject) error {
	if !kindAllowed(s.Solver.Physics, c.Kind) {
		return fmt.Errorf("constraint kind %q does not apply to %s studies", c.Kind, s.Solver.Physics)
	}
	for i := range s.constraints {
		if s.constraints[i].ID == c.ID {
			s.constraints[i] = c
			return nil
		}
	}
	return fmt.Errorf("no constraint with id %q in study %q", c.ID, s.name)
}

// RemoveConstraint deletes a constraint by id.
func (s *Study) RemoveConstraint(id string) error {
	for i := range s.constraints {
		if s.constraints[i].ID == id {
			s.constraints = append(s.constraints[:i], s.constraints[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("no constraint with id %q in study %q", id, s.name)
}

// SetPhysics switches the study's physics, resetting solver defaults and DROPPING
// constraints that no longer apply (the caller warns the user first).
func (s *Study) SetPhysics(kind PhysicsKind) []ConstraintObject {
	s.Solver = defaultSolver(kind)
	var kept, dropped []ConstraintObject
	for _, c := range s.constraints {
		if kindAllowed(kind, c.Kind) {
			kept = append(kept, c)
		} else {
			dropped = append(dropped, c)
		}
	}
	s.constraints = kept
	for i := range s.regions {
		s.regions[i].Material = defaultMaterial(kind)
	}
	return dropped
}

// clone deep-copies the study under a new id/name.
func (s *Study) clone(id, name string) *Study {
	cp := &Study{
		id: id, name: name, Solver: s.Solver, Mesh: s.Mesh,
		nextRegion: s.nextRegion, nextConstraint: s.nextConstraint, nextCoil: s.nextCoil,
	}
	cp.regions = append(cp.regions, s.regions...)
	cp.constraints = append(cp.constraints, s.constraints...)
	cp.coils = append(cp.coils, s.coils...)
	return cp
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
