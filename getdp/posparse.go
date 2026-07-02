// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

// posElement is one parsed scalar element of a GetDP .pos view: SS (tetrahedron,
// 4 nodes) or ST (triangle, 3 nodes). Values hold ALL printed steps; the last
// nodesPerElem values are the final time step.
type posElement struct {
	coords [][3]float64
	values []float64
}

// parsePosScalar reads the "View" text format GetDP prints (SS/ST scalar elements):
//
//	SS(x1,y1,z1,…,x4,y4,z4){v1,v2,v3,v4};
func parsePosScalar(path string) ([]posElement, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open pos %s: %w", path, err)
	}
	defer f.Close()
	var elems []posElement
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "SS(") || strings.HasPrefix(line, "ST(") {
			el, err := parsePosLine(line)
			if err != nil {
				return nil, fmt.Errorf("pos %s: %w", path, err)
			}
			elems = append(elems, el)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read pos %s: %w", path, err)
	}
	return elems, nil
}

// parsePosLine splits one SS/ST row into coordinates and values.
func parsePosLine(line string) (posElement, error) {
	open, close1 := strings.IndexByte(line, '('), strings.IndexByte(line, ')')
	brace, brace2 := strings.IndexByte(line, '{'), strings.IndexByte(line, '}')
	if open < 0 || close1 < open || brace < close1 || brace2 < brace {
		return posElement{}, fmt.Errorf("malformed element row %q", line)
	}
	coords, err := parseFloats(line[open+1 : close1])
	if err != nil || len(coords)%3 != 0 {
		return posElement{}, fmt.Errorf("element coordinates %q: %v", line[open+1:close1], err)
	}
	values, err := parseFloats(line[brace+1 : brace2])
	if err != nil {
		return posElement{}, fmt.Errorf("element values: %w", err)
	}
	el := posElement{values: values}
	for i := 0; i < len(coords); i += 3 {
		el.coords = append(el.coords, [3]float64{coords[i], coords[i+1], coords[i+2]})
	}
	return el, nil
}

// parseFloats splits a comma-separated float list.
func parseFloats(s string) ([]float64, error) {
	parts := strings.Split(s, ",")
	out := make([]float64, len(parts))
	for i, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

// parsePosNodalField maps a parsed .pos scalar view back onto mesh nodes by matching
// coordinates (the .pos is in metres; nodes are model units — the same modelUnitM seam
// as the writer), returning the nodal field and its range. Transient views carry one
// value set per saved step; the LAST step wins (the state the flood plot shows).
func parsePosNodalField(path string, mesh *TetMesh) (map[int]float64, float64, float64, error) {
	elems, err := parsePosScalar(path)
	if err != nil {
		return nil, 0, 0, err
	}
	if len(elems) == 0 {
		return nil, 0, 0, fmt.Errorf("pos %s holds no scalar elements", path)
	}
	byCoord := valuesByCoord(elems)
	nodal := make(map[int]float64, len(mesh.Nodes))
	lo, hi := math.Inf(1), math.Inf(-1)
	eps := posMatchEpsilon(mesh)
	for _, n := range mesh.Nodes {
		key := quantize([3]float64{n.X * modelUnitM, n.Y * modelUnitM, n.Z * modelUnitM}, eps)
		if v, ok := byCoord[key]; ok {
			nodal[n.ID] = v
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
	}
	if len(nodal) == 0 {
		return nil, 0, 0, fmt.Errorf("pos %s matched no mesh nodes (units seam mismatch?)", path)
	}
	return nodal, lo, hi, nil
}

// valuesByCoord folds the elements into a quantized coordinate → last-step value map.
func valuesByCoord(elems []posElement) map[[3]int64]float64 {
	eps := elemsEpsilon(elems)
	out := make(map[[3]int64]float64)
	for _, el := range elems {
		n := len(el.coords)
		if len(el.values) < n {
			continue
		}
		last := el.values[len(el.values)-n:] // final saved step
		for i, c := range el.coords {
			out[quantize(c, eps)] = last[i]
		}
	}
	return out
}

// posMatchEpsilon / elemsEpsilon derive the coordinate quantization grid from the
// geometry extent (1e-7 of the bbox diagonal) so writer/solver float round-trips
// still land in the same cell.
func posMatchEpsilon(mesh *TetMesh) float64 {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, n := range mesh.Nodes {
		for _, c := range []float64{n.X, n.Y, n.Z} {
			lo, hi = math.Min(lo, c*modelUnitM), math.Max(hi, c*modelUnitM)
		}
	}
	return gridEpsilon(hi - lo)
}

func elemsEpsilon(elems []posElement) float64 {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, el := range elems {
		for _, c := range el.coords {
			for k := 0; k < 3; k++ {
				lo, hi = math.Min(lo, c[k]), math.Max(hi, c[k])
			}
		}
	}
	return gridEpsilon(hi - lo)
}

func gridEpsilon(extent float64) float64 {
	if extent <= 0 {
		return 1e-12
	}
	return extent * 1e-7
}

// quantize snaps a coordinate onto the epsilon grid.
func quantize(c [3]float64, eps float64) [3]int64 {
	return [3]int64{
		int64(math.Round(c[0] / eps)),
		int64(math.Round(c[1] / eps)),
		int64(math.Round(c[2] / eps)),
	}
}

// readLastTableValue parses the value column of the LAST row of a Format Table file
// (one row per saved step; static solves have a single row).
func readLastTableValue(path string) (float64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read table %s: %w", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 2 {
		return 0, fmt.Errorf("table %s row %q, want `<step> … <value>`", path, lines[len(lines)-1])
	}
	return strconv.ParseFloat(fields[len(fields)-1], 64)
}

// tailLines returns the last n lines of a log for the monitor.
func tailLines(log string, n int) []string {
	lines := strings.Split(strings.TrimRight(log, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}
