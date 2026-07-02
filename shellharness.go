// SPDX-License-Identifier: GPL-2.0-only

package main

/*
#cgo CFLAGS: -I${SRCDIR}/include -DOBK_BUILDING_ADDIN
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include "oblikovati_addin.h"

// Fake host call table for the shell round-trip test (export_test.go): every method
// call succeeds with an empty JSON object (host-owned buffer, released via
// fake_host_free). fake_call_count lets the test wait for the async Setup goroutine
// to reach the host. This lives here and not in the test file because cgo is not
// permitted in _test.go files; the Go wrappers below keep the test cgo-free.
static int fake_call_count = 0;
static int fake_host_call(const char* m, const uint8_t* req, int n, uint8_t** resp, int* rl) {
	(void)m; (void)req; (void)n;
	__atomic_add_fetch(&fake_call_count, 1, __ATOMIC_SEQ_CST);
	*resp = (uint8_t*)malloc(2);
	memcpy(*resp, "{}", 2);
	*rl = 2;
	return OBK_OK;
}
static void fake_host_free(uint8_t* p) { free(p); }
static ObkHostCall fake_call_table(void) { return fake_host_call; }
static ObkHostFree fake_free_table(void) { return fake_host_free; }
static int fake_calls_seen(void) { return __atomic_load_n(&fake_call_count, __ATOMIC_SEQ_CST); }
*/
import "C"

// activateWithFakeHost drives ObkAddInActivate with the fake C host call table above,
// returning the C-ABI status code. Test-only seam (see the preamble comment).
func activateWithFakeHost() int {
	return int(ObkAddInActivate(C.fake_call_table(), C.fake_free_table()))
}

// fakeHostCallsSeen reports how many calls the fake host table has served.
func fakeHostCallsSeen() int { return int(C.fake_calls_seen()) }

// exportedID / exportedManifest return the C-string exports as Go strings.
func exportedID() string       { return C.GoString(ObkAddInId()) }
func exportedManifest() string { return C.GoString(ObkAddInManifest()) }

// notifyBytes forwards ev through the ObkAddInNotify C entry point.
func notifyBytes(ev []byte) int {
	return int(ObkAddInNotify((*C.uint8_t)(&ev[0]), C.int(len(ev))))
}

// deactivate / apiMajorExport / apiMinorExport wrap the remaining C entry points.
func deactivate() int     { return int(ObkAddInDeactivate()) }
func apiMajorExport() int { return int(ObkAddInApiMajor()) }
func apiMinorExport() int { return int(ObkAddInApiMinor()) }
