// Package parser 严格模式解析 Foxhole 仓库导出 .txt。
//
// 输入形如：
//
//	Umbral Wildwood - The Gap - 海港 - Public - X: 0.38 Y: 0.52,2026.06.13-06.48.16
//	Booker风暴步枪Model 838 (箱装),0
//	基础材料,260
//	（空行作为段分隔符，不语义化）
//
// 任何错误整文件 reject。详见 DESIGN.md §4。
package parser

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const crateSuffix = " (箱装)"

// Header 头行解析结果。
type Header struct {
	Region     string
	SubRegion  string
	Building   string
	Visibility string  // "Public" | "Private"
	PosX       float64
	PosY       float64
	CapturedAt time.Time // 取头行的游戏内时间戳，UTC 解释
	Raw        string
}

// Item 物品行解析结果。
type Item struct {
	Name     string // 不含 " (箱装)" 后缀
	IsCrate  bool
	Quantity int64
	LineNo   int // 1-based，原文行号，便于报错
}

// Parsed 单文件解析结果（严格模式：错误时返回 nil + err）。
type Parsed struct {
	Header Header
	Items  []Item
}

// Error 是结构化错误，前端可拿到 LineNo / Field 显示。
type Error struct {
	LineNo  int
	Field   string
	Message string
}

func (e *Error) Error() string {
	if e.LineNo > 0 {
		return fmt.Sprintf("line %d (%s): %s", e.LineNo, e.Field, e.Message)
	}
	return fmt.Sprintf("(%s): %s", e.Field, e.Message)
}

// Parse 严格模式解析。
func Parse(raw string) (*Parsed, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, &Error{Field: "file", Message: "empty"}
	}

	// 统一行尾，但不修改其他字符。
	text := strings.ReplaceAll(raw, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	// 去掉可能的 BOM。
	text = strings.TrimPrefix(text, "\ufeff")

	lines := strings.Split(text, "\n")

	// 找到第一条非空行作为 header。
	var headerLine string
	var headerIdx int
	for i, ln := range lines {
		if strings.TrimSpace(ln) != "" {
			headerLine = ln
			headerIdx = i
			break
		}
	}
	if headerLine == "" {
		return nil, &Error{Field: "header", Message: "missing"}
	}

	header, err := parseHeader(headerLine)
	if err != nil {
		return nil, err
	}

	items := make([]Item, 0, len(lines))
	// 用 (name, is_crate) 作为唯一键去重，"取最后一行"。
	type key struct {
		name    string
		isCrate bool
	}
	indexOf := map[key]int{}

	for i := headerIdx + 1; i < len(lines); i++ {
		raw := lines[i]
		if strings.TrimSpace(raw) == "" {
			continue
		}
		itm, err := parseItemLine(raw, i+1)
		if err != nil {
			return nil, err
		}
		k := key{itm.Name, itm.IsCrate}
		if idx, ok := indexOf[k]; ok {
			items[idx] = itm
		} else {
			indexOf[k] = len(items)
			items = append(items, itm)
		}
	}

	if len(items) == 0 {
		return nil, &Error{Field: "items", Message: "no item rows"}
	}

	return &Parsed{Header: header, Items: items}, nil
}

func parseHeader(line string) (Header, error) {
	h := Header{Raw: line}
	// 1) 从右找最后一个逗号 → 时间戳；左半是位置信息行。
	commaIdx := strings.LastIndex(line, ",")
	if commaIdx < 0 {
		return h, &Error{LineNo: 1, Field: "header", Message: "no comma before timestamp"}
	}
	left := strings.TrimSpace(line[:commaIdx])
	tsStr := strings.TrimSpace(line[commaIdx+1:])

	t, err := time.Parse("2006.01.02-15.04.05", tsStr)
	if err != nil {
		return h, &Error{LineNo: 1, Field: "header.timestamp", Message: fmt.Sprintf("invalid %q", tsStr)}
	}
	h.CapturedAt = t

	// 2) 在左半按 " - " 切，至少 5 段：region / sub_region / building... / visibility / "X: f Y: f"
	parts := strings.Split(left, " - ")
	if len(parts) < 5 {
		return h, &Error{LineNo: 1, Field: "header", Message: fmt.Sprintf("expect ≥5 ' - ' parts, got %d", len(parts))}
	}
	coord := parts[len(parts)-1]
	vis := strings.TrimSpace(parts[len(parts)-2])
	region := parts[0]
	subRegion := parts[1]
	// 中间任意多段都拼回 building（建筑名内可能含 ' - '）。
	building := strings.Join(parts[2:len(parts)-2], " - ")
	if region == "" || subRegion == "" || building == "" {
		return h, &Error{LineNo: 1, Field: "header", Message: "empty region/sub_region/building"}
	}
	if vis != "Public" && vis != "Private" {
		return h, &Error{LineNo: 1, Field: "header.visibility", Message: fmt.Sprintf("expect Public|Private, got %q", vis)}
	}

	// 3) 坐标段："X: f Y: f"
	if !strings.HasPrefix(coord, "X:") {
		return h, &Error{LineNo: 1, Field: "header", Message: "missing 'X:' in tail"}
	}
	tail := coord[len("X:"):]
	yIdx := strings.Index(tail, " Y:")
	if yIdx < 0 {
		return h, &Error{LineNo: 1, Field: "header", Message: "missing ' Y:'"}
	}
	xStr := strings.TrimSpace(tail[:yIdx])
	yStr := strings.TrimSpace(tail[yIdx+len(" Y:"):])
	x, err := strconv.ParseFloat(xStr, 64)
	if err != nil {
		return h, &Error{LineNo: 1, Field: "header.x", Message: fmt.Sprintf("invalid %q", xStr)}
	}
	y, err := strconv.ParseFloat(yStr, 64)
	if err != nil {
		return h, &Error{LineNo: 1, Field: "header.y", Message: fmt.Sprintf("invalid %q", yStr)}
	}

	h.Region = region
	h.SubRegion = subRegion
	h.Building = building
	h.Visibility = vis
	h.PosX = x
	h.PosY = y
	return h, nil
}

func parseItemLine(line string, lineNo int) (Item, error) {
	commaIdx := strings.LastIndex(line, ",")
	if commaIdx < 0 {
		return Item{}, &Error{LineNo: lineNo, Field: "item", Message: "no comma"}
	}
	namePart := line[:commaIdx]
	qtyPart := strings.TrimSpace(line[commaIdx+1:])

	qty, err := strconv.ParseInt(qtyPart, 10, 64)
	if err != nil {
		return Item{}, &Error{LineNo: lineNo, Field: "item.quantity", Message: fmt.Sprintf("invalid %q", qtyPart)}
	}
	if qty < 0 {
		return Item{}, &Error{LineNo: lineNo, Field: "item.quantity", Message: "negative"}
	}

	isCrate := false
	if strings.HasSuffix(namePart, crateSuffix) {
		isCrate = true
		namePart = strings.TrimSuffix(namePart, crateSuffix)
	}
	if strings.TrimSpace(namePart) == "" {
		return Item{}, &Error{LineNo: lineNo, Field: "item.name", Message: "empty"}
	}
	return Item{
		Name:     namePart,
		IsCrate:  isCrate,
		Quantity: qty,
		LineNo:   lineNo,
	}, nil
}

// IsParseError 判断 err 是否是解析器结构化错误。
func IsParseError(err error) bool {
	var pe *Error
	return errors.As(err, &pe)
}