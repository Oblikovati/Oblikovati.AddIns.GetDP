// SPDX-License-Identifier: GPL-2.0-only

package demos

// rect is an axis-aligned rectangle profile in host parameter expressions: the lower-left
// corner (x,y) and the size (w,h). Anchoring at the origin ("0","0") keeps the rectangle
// parameter-driven; a non-origin anchor pins the corner at its evaluated position.
type rect struct{ x, y, w, h string }

// extrudeRectangleFeature lays one rect on a fresh XY sketch and extrudes it, returning the
// host feature name (so a fin can seed a pattern). operation is "new" or "join".
func extrudeRectangleFeature(a Author, r rect, distanceExpr, operation string) (string, error) {
	sk, err := a.Sketch("XY")
	if err != nil {
		return "", err
	}
	if err := a.CornerRectangle(sk, r.x, r.y, r.w, r.h); err != nil {
		return "", err
	}
	return a.Extrude(sk, distanceExpr, operation)
}

// extrudeRectangle is extrudeRectangleFeature for callers that do not need the feature name.
func extrudeRectangle(a Author, r rect, distanceExpr, operation string) error {
	_, err := extrudeRectangleFeature(a, r, distanceExpr, operation)
	return err
}
