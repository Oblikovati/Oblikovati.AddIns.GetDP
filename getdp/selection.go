// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"encoding/base64"
	"strings"
)

// faceRefPrefix is how the host's selection encodes a face: "face/" + URL-safe base64 of
// the raw reference key. face.calculateFacets, by contrast, resolves the RAW key bytes —
// so a selection reference must be decoded before it is used to address a face.
const faceRefPrefix = "face/"

// decodeSelectedFaces keeps only the face references in a selection and decodes each into
// the raw reference key face.calculateFacets resolves. Non-face references (edges,
// vertices, work geometry, …) are dropped.
func decodeSelectedFaces(refs []string) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if key, ok := decodeFaceRef(ref); ok {
			out = append(out, key)
		}
	}
	return out
}

// decodeFaceRef turns a "face/<url-base64>" selection reference into its raw key, or
// reports false for a non-face / malformed reference.
func decodeFaceRef(ref string) (string, bool) {
	if !strings.HasPrefix(ref, faceRefPrefix) {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(ref, faceRefPrefix))
	if err != nil {
		return "", false
	}
	return string(raw), true
}
