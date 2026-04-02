package hashline

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"os"
	"strconv"
	"strings"

	"github.com/AlleyBo55/gocode/internal/toolimpl"
)

// ContentHash computes a deterministic 2-character base36 hash of trimmed line content.
// Uses CRC32 of the trimmed content, encoded as base36, truncated/padded to 2 characters.
func ContentHash(line string) string {
	trimmed := strings.TrimSpace(line)
	checksum := crc32.ChecksumIEEE([]byte(trimmed))
	// Mod by 36^2 = 1296 to get a value in [0, 1295], then encode as 2-char base36.
	val := checksum % 1296
	encoded := strconv.FormatUint(uint64(val), 36)
	// Pad to 2 characters with leading zero if needed.
	if len(encoded) < 2 {
		encoded = "0" + encoded
	}
	return encoded
}

// AnnotateLines takes raw file lines and returns hash-annotated output in
// "LINE#HASH| content" format where LINE is 1-indexed.
func AnnotateLines(lines []string) string {
	var sb strings.Builder
	for i, line := range lines {
		hash := ContentHash(line)
		sb.WriteString(fmt.Sprintf("%d#%s| %s\n", i+1, hash, line))
	}
	return sb.String()
}

// EditRef represents a line reference in an edit request.
type EditRef struct {
	LineNumber int    // 1-indexed
	Hash       string // expected 2-char hash
}

// ValidateEdit checks that each referenced line's current content hash matches
// the hash provided in the edit request. Returns nil if all match, or an error
// identifying the first mismatched line.
func ValidateEdit(currentLines []string, editRefs []EditRef) error {
	for _, ref := range editRefs {
		if ref.LineNumber < 1 || ref.LineNumber > len(currentLines) {
			return fmt.Errorf("line %d: out of range (file has %d lines)", ref.LineNumber, len(currentLines))
		}
		actual := ContentHash(currentLines[ref.LineNumber-1])
		if actual != ref.Hash {
			return fmt.Errorf("line %d: hash mismatch (expected %s, got %s)", ref.LineNumber, ref.Hash, actual)
		}
	}
	return nil
}

// HashReadTool wraps the existing FileReadTool, annotating output with hashes.
// Implements toolimpl.ToolExecutor.
type HashReadTool struct {
	Inner toolimpl.ToolExecutor // the original FileReadTool
}

// Execute delegates to the inner FileReadTool, then annotates the output lines
// with per-line content hashes in "LINE#HASH| content" format.
func (h *HashReadTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	result := h.Inner.Execute(params)
	if !result.Success || result.Output == "" {
		return result
	}

	// The inner FileReadTool output is formatted as "N: content\n" per line.
	// We need to strip the existing line numbers, then re-annotate with hashes.
	rawLines := strings.Split(strings.TrimSuffix(result.Output, "\n"), "\n")
	var contentLines []string
	lineNumbers := make([]int, 0, len(rawLines))

	for _, raw := range rawLines {
		// Parse "N: content" format from FileReadTool
		colonIdx := strings.Index(raw, ": ")
		if colonIdx > 0 {
			numStr := raw[:colonIdx]
			if n, err := strconv.Atoi(numStr); err == nil {
				lineNumbers = append(lineNumbers, n)
				contentLines = append(contentLines, raw[colonIdx+2:])
				continue
			}
		}
		// Fallback: treat the whole line as content
		lineNumbers = append(lineNumbers, len(contentLines)+1)
		contentLines = append(contentLines, raw)
	}

	// Re-annotate with hashes using original line numbers
	var sb strings.Builder
	for i, content := range contentLines {
		hash := ContentHash(content)
		sb.WriteString(fmt.Sprintf("%d#%s| %s\n", lineNumbers[i], hash, content))
	}

	return toolimpl.ToolResult{Success: true, Output: sb.String()}
}

// HashEditTool wraps the existing FileEditTool, validating hashes before applying.
// Implements toolimpl.ToolExecutor.
type HashEditTool struct {
	Inner toolimpl.ToolExecutor // the original FileEditTool
}

// Execute validates hash references against the current file content before
// delegating to the inner FileEditTool. If hash_refs is present in params,
// each referenced line's hash is checked. On mismatch, the edit is rejected.
// If hash_refs is absent, the edit is passed through directly.
func (h *HashEditTool) Execute(params map[string]interface{}) toolimpl.ToolResult {
	// Check for hash_refs param
	hashRefsRaw, hasRefs := params["hash_refs"]
	if !hasRefs {
		return h.Inner.Execute(params)
	}

	// Parse hash_refs: expects a JSON array of {line_number, hash} objects.
	refs, err := parseHashRefs(hashRefsRaw)
	if err != nil {
		return toolimpl.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("invalid hash_refs: %v", err),
		}
	}

	if len(refs) == 0 {
		return h.Inner.Execute(params)
	}

	// Read the current file to validate hashes
	path, _ := params["path"].(string)
	if path == "" {
		return toolimpl.ToolResult{Success: false, Error: "missing required param: path"}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return toolimpl.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("reading file for hash validation: %v", err),
		}
	}

	currentLines := strings.Split(string(data), "\n")
	if err := ValidateEdit(currentLines, refs); err != nil {
		return toolimpl.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("hash validation failed: %v", err),
		}
	}

	// Strip hash_refs from params before passing to inner tool
	cleanParams := make(map[string]interface{}, len(params))
	for k, v := range params {
		if k != "hash_refs" {
			cleanParams[k] = v
		}
	}

	return h.Inner.Execute(cleanParams)
}

// parseHashRefs parses the hash_refs parameter into a slice of EditRef.
// Accepts a JSON string, a []interface{} (already parsed), or nil.
func parseHashRefs(raw interface{}) ([]EditRef, error) {
	switch v := raw.(type) {
	case string:
		var refs []EditRef
		if err := json.Unmarshal([]byte(v), &refs); err != nil {
			return nil, fmt.Errorf("parsing hash_refs JSON string: %v", err)
		}
		return refs, nil
	case []interface{}:
		refs := make([]EditRef, 0, len(v))
		for i, item := range v {
			m, ok := item.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("hash_refs[%d]: expected object, got %T", i, item)
			}
			ref, err := parseEditRefMap(m)
			if err != nil {
				return nil, fmt.Errorf("hash_refs[%d]: %v", i, err)
			}
			refs = append(refs, ref)
		}
		return refs, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported hash_refs type: %T", raw)
	}
}

// parseEditRefMap extracts an EditRef from a map[string]interface{}.
func parseEditRefMap(m map[string]interface{}) (EditRef, error) {
	var ref EditRef

	switch ln := m["line_number"].(type) {
	case float64:
		ref.LineNumber = int(ln)
	case json.Number:
		n, err := ln.Int64()
		if err != nil {
			return ref, fmt.Errorf("invalid line_number: %v", err)
		}
		ref.LineNumber = int(n)
	default:
		return ref, fmt.Errorf("missing or invalid line_number")
	}

	hash, ok := m["hash"].(string)
	if !ok || hash == "" {
		return ref, fmt.Errorf("missing or invalid hash")
	}
	ref.Hash = hash

	return ref, nil
}

// RegisterHashlineTools replaces the filereadtool and fileedittool entries in
// the registry with hash-annotating/validating wrappers around the originals.
func RegisterHashlineTools(r *toolimpl.Registry) {
	origRead := r.Get("filereadtool")
	origEdit := r.Get("fileedittool")

	if origRead != nil {
		r.Set("filereadtool", &HashReadTool{Inner: origRead})
	}
	if origEdit != nil {
		r.Set("fileedittool", &HashEditTool{Inner: origEdit})
	}
}
