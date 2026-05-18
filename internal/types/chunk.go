package types

import (
	"strconv"
	"strings"
)

func ChunkKeys(keys []string, size int) [][]string {
	n := len(keys)
	if n == 0 {
		return nil
	}
	if size <= 0 {
		return [][]string{keys}
	}
	nchunks := (n + size - 1) / size
	chunks := make([][]string, nchunks)
	for c, i := 0, 0; c < nchunks; c++ {
		end := i + size
		if end > n {
			end = n
		}
		chunks[c] = keys[i:end]
		i = end
	}
	return chunks
}

// appendINPlaceholderList writes count comma-separated @p placeholders starting at
// startIndex into b. A fixed scratch buffer avoids per-index string allocations from
// strconv.Itoa / fmt when expanding large IN lists.
func appendINPlaceholderList(b *strings.Builder, startIndex, count int) {
	if count <= 0 {
		return
	}
	var scratch [32]byte
	for i := 0; i < count; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString("@p")
		b.Write(strconv.AppendInt(scratch[:0], int64(startIndex+i), 10))
	}
}

func placeholderListGrowEstimate(count int) int {
	if count <= 0 {
		return 0
	}
	return count*8 + 8
}

func BuildINQuery(template, placeholder string, keys []string, startIndex int) (string, []string) {
	n := len(keys)
	if n == 0 {
		return strings.Replace(template, placeholder, "", -1), keys
	}
	if !strings.Contains(template, placeholder) {
		return template, keys
	}
	var out strings.Builder
	occ := strings.Count(template, placeholder)
	out.Grow(len(template) + occ*(placeholderListGrowEstimate(n)-len(placeholder)))
	pos := 0
	for {
		i := strings.Index(template[pos:], placeholder)
		if i < 0 {
			out.WriteString(template[pos:])
			break
		}
		i += pos
		out.WriteString(template[pos:i])
		appendINPlaceholderList(&out, startIndex, n)
		pos = i + len(placeholder)
	}
	return out.String(), keys
}

func buildDualINQueryFallback(template, placeholder1 string, keys1 []string, placeholder2 string, keys2 []string, startIndex int) string {
	var b1, b2 strings.Builder
	if n1 := len(keys1); n1 > 0 {
		b1.Grow(placeholderListGrowEstimate(n1))
	}
	if n2 := len(keys2); n2 > 0 {
		b2.Grow(placeholderListGrowEstimate(n2))
	}
	appendINPlaceholderList(&b1, startIndex, len(keys1))
	appendINPlaceholderList(&b2, startIndex+len(keys1), len(keys2))

	q := strings.Replace(template, placeholder1, b1.String(), -1)
	return strings.Replace(q, placeholder2, b2.String(), -1)
}

type dualINTemplatePlan struct {
	segments []string
	kinds    []byte
	occ1     int
	occ2     int
}

func buildDualINTemplatePlan(template, placeholder1, placeholder2 string) dualINTemplatePlan {
	plan := dualINTemplatePlan{
		segments: make([]string, 0, 8),
		kinds:    make([]byte, 0, 8),
	}
	pos := 0
	for pos < len(template) {
		next1 := strings.Index(template[pos:], placeholder1)
		next2 := strings.Index(template[pos:], placeholder2)
		switch {
		case next1 < 0 && next2 < 0:
			plan.segments = append(plan.segments, template[pos:])
			return plan
		case next2 < 0 || (next1 >= 0 && next1 < next2):
			plan.segments = append(plan.segments, template[pos:pos+next1])
			plan.kinds = append(plan.kinds, 1)
			plan.occ1++
			pos += next1 + len(placeholder1)
		default:
			plan.segments = append(plan.segments, template[pos:pos+next2])
			plan.kinds = append(plan.kinds, 2)
			plan.occ2++
			pos += next2 + len(placeholder2)
		}
	}
	plan.segments = append(plan.segments, "")
	return plan
}

type DualINTemplate struct {
	plan            dualINTemplatePlan
	templateLen     int
	placeholder1Len int
	placeholder2Len int
}

// CompileDualINTemplate parses a SQL template once so repeated calls can skip
// rescanning placeholder positions. Useful for hot paths with stable query text
// such as inspector object and column lookups.
func CompileDualINTemplate(template, placeholder1, placeholder2 string) DualINTemplate {
	return DualINTemplate{
		templateLen:     len(template),
		placeholder1Len: len(placeholder1),
		placeholder2Len: len(placeholder2),
		plan:            buildDualINTemplatePlan(template, placeholder1, placeholder2),
	}
}

func buildDualINQueryFromPlan(plan dualINTemplatePlan, templateLen, placeholder1Len, placeholder2Len int, keys1 []string, keys2 []string, startIndex int) string {
	n1 := len(keys1)
	n2 := len(keys2)
	var q strings.Builder
	q.Grow(templateLen +
		plan.occ1*(placeholderListGrowEstimate(n1)-placeholder1Len) +
		plan.occ2*(placeholderListGrowEstimate(n2)-placeholder2Len))
	for i, seg := range plan.segments {
		q.WriteString(seg)
		if i >= len(plan.kinds) {
			continue
		}
		switch plan.kinds[i] {
		case 1:
			appendINPlaceholderList(&q, startIndex, n1)
		case 2:
			appendINPlaceholderList(&q, startIndex+n1, n2)
		}
	}
	return q.String()
}

func (t DualINTemplate) BuildQuery(keys1 []string, keys2 []string, startIndex int) string {
	return buildDualINQueryFromPlan(t.plan, t.templateLen, t.placeholder1Len, t.placeholder2Len, keys1, keys2, startIndex)
}

func (t DualINTemplate) Build(keys1 []string, keys2 []string, startIndex int) (string, []string) {
	n1 := len(keys1)
	n2 := len(keys2)
	switch {
	case n1 == 0:
		return t.BuildQuery(keys1, keys2, startIndex), keys2
	case n2 == 0:
		return t.BuildQuery(keys1, keys2, startIndex), keys1
	}
	args := make([]string, n1+n2)
	copy(args, keys1)
	copy(args[n1:], keys2)
	return t.BuildQuery(keys1, keys2, startIndex), args
}

func combineStringArgs(keys1 []string, keys2 []string) []string {
	switch {
	case len(keys1) == 0:
		return keys2
	case len(keys2) == 0:
		return keys1
	}
	args := make([]string, len(keys1)+len(keys2))
	copy(args, keys1)
	copy(args[len(keys1):], keys2)
	return args
}

// BuildDualINQuery expands two IN placeholders with consecutive @p indices and
// returns a single string-args slice sized len(keys1)+len(keys2). String->any
// boxing moves to the driver boundary via driver.Conn.QueryStringsContext.
func BuildDualINQuery(template, placeholder1 string, keys1 []string, placeholder2 string, keys2 []string, startIndex int) (string, []string) {
	if placeholder1 == placeholder2 {
		return buildDualINQueryFallback(template, placeholder1, keys1, placeholder2, keys2, startIndex), combineStringArgs(keys1, keys2)
	}

	plan := buildDualINTemplatePlan(template, placeholder1, placeholder2)
	return buildDualINQueryFromPlan(plan, len(template), len(placeholder1), len(placeholder2), keys1, keys2, startIndex), combineStringArgs(keys1, keys2)
}
