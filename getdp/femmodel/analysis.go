// SPDX-License-Identifier: GPL-2.0-only

// Package femmodel is the study-model aggregate of the GetDP add-in: the single source
// of truth the browser tree and task panels project from and the study runner consumes
// (same role as the reference add-in's femmodel, but multi-study: a document holds N
// studies, exactly one active). Mutators preserve the invariants — unique ids, exactly
// one active study, every body in at most one region per study.
package femmodel

import "fmt"

// Analysis is the root aggregate: the document's studies, exactly one active.
type Analysis struct {
	studies   []*Study
	active    int
	nextStudy int
}

// NewAnalysis returns an aggregate seeded with one default electrokinetic study —
// the tree is never empty, so every panel has something to edit.
func NewAnalysis() *Analysis {
	a := &Analysis{}
	a.AddStudy(PhysicsElectrokinetics)
	return a
}

// Studies returns the studies in creation order.
func (a *Analysis) Studies() []*Study { return a.studies }

// Active returns the active study (never nil: the aggregate always holds ≥1 study).
func (a *Analysis) Active() *Study { return a.studies[a.active] }

// AddStudy appends a study of the given physics with per-physics defaults and makes it
// active. Names are unique by construction ("Study N — <physics>").
func (a *Analysis) AddStudy(kind PhysicsKind) *Study {
	a.nextStudy++
	s := newStudy(fmt.Sprintf("study%d", a.nextStudy),
		fmt.Sprintf("Study %d — %s", a.nextStudy, kind), kind)
	a.studies = append(a.studies, s)
	a.active = len(a.studies) - 1
	return s
}

// DuplicateStudy deep-copies the study with the given id, appends the copy (made
// active) and returns it. Returns an error for an unknown id.
func (a *Analysis) DuplicateStudy(id string) (*Study, error) {
	src, err := a.StudyByID(id)
	if err != nil {
		return nil, err
	}
	a.nextStudy++
	cp := src.clone(fmt.Sprintf("study%d", a.nextStudy), src.name+" (copy)")
	a.studies = append(a.studies, cp)
	a.active = len(a.studies) - 1
	return cp, nil
}

// RemoveStudy deletes the study with the given id. The last study cannot be removed
// (the aggregate invariant is ≥1 study); if the active study is removed, the first
// remaining study becomes active.
func (a *Analysis) RemoveStudy(id string) error {
	if len(a.studies) == 1 {
		return fmt.Errorf("cannot remove study %q: a document keeps at least one study", id)
	}
	for i, s := range a.studies {
		if s.id == id {
			a.studies = append(a.studies[:i], a.studies[i+1:]...)
			if a.active >= len(a.studies) {
				a.active = 0
			}
			return nil
		}
	}
	return fmt.Errorf("no study with id %q (have %d studies)", id, len(a.studies))
}

// SetActive makes the study with the given id active.
func (a *Analysis) SetActive(id string) error {
	for i, s := range a.studies {
		if s.id == id {
			a.active = i
			return nil
		}
	}
	return fmt.Errorf("no study with id %q (have %d studies)", id, len(a.studies))
}

// StudyByID resolves a study by id.
func (a *Analysis) StudyByID(id string) (*Study, error) {
	for _, s := range a.studies {
		if s.id == id {
			return s, nil
		}
	}
	return nil, fmt.Errorf("no study with id %q (have %d studies)", id, len(a.studies))
}
