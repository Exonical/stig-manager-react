package xccdf

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// zipMagic is the leading four bytes of every zip archive.
var zipMagic = []byte{0x50, 0x4b, 0x03, 0x04}

// maxNestedZipDepth bounds the recursion the importer will perform
// when a STIG zip contains nested zips (e.g. DISA outer zip → inner
// `_xccdf.zip`).
const maxNestedZipDepth = 2

// maxDecompressedEntryBytes caps the decompressed size of any single
// zip entry. Legitimate DISA XCCDF files are well under 10 MiB; the
// 128 MiB ceiling is wide enough to absorb future growth while still
// preventing a zip-bomb (small compressed payload, huge decompressed
// payload) from exhausting memory.
//
// Declared as a var so tests can lower it to verify the defence
// without having to construct hundreds of MiB of compressible bytes.
var maxDecompressedEntryBytes int64 = 128 << 20

// IsZip returns true when payload begins with the zip local-file-
// header magic.
func IsZip(payload []byte) bool {
	return len(payload) >= 4 && bytes.HasPrefix(payload, zipMagic)
}

// ExtractBenchmarks returns every parsed XCCDF Benchmark contained in
// payload. Three input shapes are accepted:
//
//   - raw XCCDF XML (parsed directly).
//   - a zip whose entries include one or more `*xccdf.xml` files (DISA
//     "Manual STIG" bundle layout, e.g.
//     `U_RHEL_10_V1R1_STIG.zip`).
//   - a zip whose entries include a nested `*xccdf.zip` file, which is
//     itself walked for `*xccdf.xml` (recurses once; matches the
//     DISA "STIG Library" quarterly bundle layout).
//
// Each returned Benchmark is paired with the inner-file basename it
// came from so the caller can surface friendly error messages and
// support partial-failure reporting.
func ExtractBenchmarks(payload []byte) ([]Extracted, error) {
	if len(payload) == 0 {
		return nil, errors.New("upload is empty")
	}
	if !IsZip(payload) {
		bench, err := Parse(bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("parse xccdf: %w", err)
		}
		return []Extracted{{Benchmark: bench, Source: "upload.xml"}}, nil
	}
	out, err := extractFromZip(payload, 0)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("zip contains no XCCDF Benchmark file")
	}
	return out, nil
}

// Extracted bundles a parsed Benchmark with the name of the file it
// came from inside the source archive.
type Extracted struct {
	Benchmark *Benchmark
	Source    string
}

func extractFromZip(payload []byte, depth int) ([]Extracted, error) {
	if depth > maxNestedZipDepth {
		return nil, fmt.Errorf("nested zip depth exceeded (%d)", maxNestedZipDepth)
	}
	zr, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	var (
		results    []Extracted
		nestedZips []*zip.File
	)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := strings.ToLower(path.Base(f.Name))
		switch {
		case strings.HasSuffix(base, "xccdf.xml"), strings.HasSuffix(base, "-xccdf.xml"), strings.HasSuffix(base, "_xccdf.xml"):
			ext, err := readZipEntry(f)
			if err != nil {
				return nil, fmt.Errorf("read %q: %w", f.Name, err)
			}
			bench, err := Parse(bytes.NewReader(ext))
			if err != nil {
				return nil, fmt.Errorf("parse %q: %w", f.Name, err)
			}
			results = append(results, Extracted{Benchmark: bench, Source: f.Name})
		case strings.HasSuffix(base, "xccdf.zip"), strings.HasSuffix(base, "_xccdf.zip"), strings.HasSuffix(base, "-xccdf.zip"):
			nestedZips = append(nestedZips, f)
		}
	}
	if len(results) > 0 {
		return results, nil
	}
	// No direct xccdf.xml at this depth — walk into nested zips.
	for _, nz := range nestedZips {
		inner, err := readZipEntry(nz)
		if err != nil {
			return nil, fmt.Errorf("read nested zip %q: %w", nz.Name, err)
		}
		nested, err := extractFromZip(inner, depth+1)
		if err != nil {
			return nil, fmt.Errorf("nested %q: %w", nz.Name, err)
		}
		results = append(results, nested...)
	}
	return results, nil
}

// readZipEntry reads a single zip File into memory, capping the
// decompressed size at maxDecompressedEntryBytes to defend against
// zip-bomb uploads where a small compressed payload expands to many
// gigabytes when read out of the archive.
//
// The UncompressedSize64 header is consulted as a cheap fast-reject,
// but the hard guarantee comes from wrapping the reader with an
// io.LimitReader (+1 sentinel byte) and rejecting anything that
// actually attempts to overrun the cap.
func readZipEntry(f *zip.File) ([]byte, error) {
	limit := maxDecompressedEntryBytes
	if int64(f.UncompressedSize64) > limit {
		return nil, fmt.Errorf("zip entry %q exceeds %d-byte decompressed cap (declared %d)",
			f.Name, limit, f.UncompressedSize64)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	limited := io.LimitReader(rc, limit+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("zip entry %q exceeds %d-byte decompressed cap",
			f.Name, limit)
	}
	return data, nil
}
