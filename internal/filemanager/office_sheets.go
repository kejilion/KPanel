package filemanager

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

var officeNumeric = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

func (p *officePackage) recalculatePatch() officePatch {
	x := p.xml["xl/workbook.xml"]
	if n := x.root.child(nsSheet, "calcPr"); n != nil {
		op, name := officeOpening(x.data, n)
		op = officeRemoveAttrs(op, "fullCalcOnLoad", "forceFullCalc")
		op = strings.TrimSuffix(strings.TrimSuffix(op, ">"), "/")
		return officePatch{n.start, n.end, op + ` fullCalcOnLoad="1" forceFullCalc="1"></` + name + ">"}
	}
	_, name := officeOpening(x.data, x.root)
	prefix := ""
	if i := strings.IndexByte(name, ':'); i >= 0 {
		prefix = name[:i+1]
	}
	position := x.root.closeStart
	// CT_Workbook places calcPr before these optional trailing elements.
	afterCalc := map[string]bool{"oleSize": true, "customWorkbookViews": true, "pivotCaches": true, "smartTagPr": true, "smartTagTypes": true, "webPublishing": true, "fileRecoveryPr": true, "webPublishObjects": true, "extLst": true}
	for _, child := range x.root.children {
		if child.name.Space == nsSheet && afterCalc[child.name.Local] {
			position = child.start
			break
		}
	}
	return officePatch{position, position, "<" + prefix + `calcPr fullCalcOnLoad="1" forceFullCalc="1"/>`}
}

func (p *officePackage) sheets() ([]contract.OfficeSection, error) {
	x, err := p.part("xl/workbook.xml")
	if err != nil {
		return nil, err
	}
	if !x.root.is(nsSheet, "workbook") {
		return nil, ErrOfficeUnsupported
	}
	rels, err := p.relationships("xl/workbook.xml")
	if err != nil {
		return nil, err
	}
	var shared []string
	var sharedPlain []bool
	if p.files["xl/sharedStrings.xml"] != nil {
		s, err := p.part("xl/sharedStrings.xml")
		if err != nil {
			return nil, err
		}
		if !s.root.is(nsSheet, "sst") {
			return nil, ErrOfficeUnsupported
		}
		for _, n := range s.root.children {
			if n.is(nsSheet, "si") {
				shared = append(shared, officeString(n, nsSheet))
				sharedPlain = append(sharedPlain, len(n.children) == 1 && n.children[0].is(nsSheet, "t"))
			}
		}
	}
	var sections []contract.OfficeSection
	seenParts := map[string]bool{}
	for _, sheet := range x.root.all(nsSheet, "sheet") {
		if len(sections) >= 128 {
			return nil, ErrTooLarge
		}
		part := rels[sheet.relID()]
		if part == "" || seenParts[part] {
			return nil, ErrOfficeUnsupported
		}
		seenParts[part] = true
		s, err := p.part(part)
		if err != nil {
			return nil, err
		}
		if !s.root.is(nsSheet, "worksheet") {
			return nil, ErrOfficeUnsupported
		}
		section := contract.OfficeSection{Name: sheet.attr("name"), Items: []contract.OfficeItem{}}
		if err := p.displayText(section.Name); err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		protected := s.root.child(nsSheet, "sheetProtection") != nil
		var formulaRanges []officeRange
		for _, formula := range s.root.all(nsSheet, "f") {
			if formula.attr("t") != "array" && formula.attr("t") != "dataTable" {
				continue
			}
			rangeValue, ok := officeFormulaRange(formula.attr("ref"))
			if !ok {
				return nil, ErrOfficeUnsupported
			}
			formulaRanges = append(formulaRanges, rangeValue)
			if len(formulaRanges) > 1024 {
				return nil, ErrTooLarge
			}
		}
		for _, cell := range s.root.all(nsSheet, "c") {
			ref := cell.attr("r")
			row, col, ok := officeCellPosition(ref)
			if !ok || seen[ref] {
				return nil, ErrOfficeUnsupported
			}
			seen[ref] = true
			id, err := p.itemID(part, cell)
			if err != nil {
				return nil, err
			}
			item := contract.OfficeItem{ID: id, Kind: "cell", Row: row, Column: col}
			plain := cell.attr("cm") == "" && cell.attr("vm") == ""
			for _, formulaRange := range formulaRanges {
				if formulaRange.contains(row, col) {
					plain = false
					break
				}
			}
			v := cell.child(nsSheet, "v")
			if v != nil {
				item.Text = v.text
			}
			switch cell.attr("t") {
			case "s":
				index, err := strconv.Atoi(item.Text)
				if err != nil || index < 0 || index >= len(shared) {
					return nil, ErrOfficeUnsupported
				}
				item.Text = shared[index]
				plain = plain && sharedPlain[index]
			case "inlineStr":
				item.Text = officeString(cell, nsSheet)
				inline := cell.child(nsSheet, "is")
				plain = plain && inline != nil && len(inline.children) == 1 && inline.children[0].is(nsSheet, "t")
			case "b":
				if item.Text == "1" {
					item.Text = "TRUE"
				} else {
					item.Text = "FALSE"
				}
			}
			if f := cell.child(nsSheet, "f"); f != nil {
				item.Formula = "=" + f.text
			}
			if err := p.displayText(item.Text, item.Formula); err != nil {
				return nil, err
			}
			item.Editable = plain && !protected && item.Formula == "" && (cell.attr("t") == "" || cell.attr("t") == "n" || cell.attr("t") == "s" || cell.attr("t") == "inlineStr")
			for _, child := range cell.children {
				if !child.is(nsSheet, "v") && !child.is(nsSheet, "is") {
					item.Editable = false
				}
			}
			if item.Editable {
				p.targets[id] = officeTarget{part: part, cell: cell, text: item.Text}
			}
			section.Rows = max(section.Rows, row)
			section.Columns = max(section.Columns, col)
			section.Items = append(section.Items, item)
		}
		sections = append(sections, section)
	}
	if len(sections) == 0 {
		return nil, ErrOfficeUnsupported
	}
	return sections, nil
}

type officeRange struct{ firstRow, lastRow, firstColumn, lastColumn int }

func (r officeRange) contains(row, column int) bool {
	return row >= r.firstRow && row <= r.lastRow && column >= r.firstColumn && column <= r.lastColumn
}
func officeFormulaRange(ref string) (officeRange, bool) {
	values := strings.Split(ref, ":")
	if len(values) > 2 {
		return officeRange{}, false
	}
	row, col, ok := officeCellPosition(values[0])
	if !ok {
		return officeRange{}, false
	}
	r := officeRange{row, row, col, col}
	if len(values) == 2 {
		lastRow, lastColumn, ok := officeCellPosition(values[1])
		if !ok || lastRow < row || lastColumn < col {
			return officeRange{}, false
		}
		r.lastRow, r.lastColumn = lastRow, lastColumn
	}
	return r, true
}

func officeCellPosition(ref string) (int, int, bool) {
	column, i := 0, 0
	for i < len(ref) && ref[i] >= 'A' && ref[i] <= 'Z' {
		column = column*26 + int(ref[i]-'A'+1)
		if column > 16384 {
			return 0, 0, false
		}
		i++
	}
	if i == len(ref) || ref[i] < '1' || ref[i] > '9' {
		return 0, 0, false
	}
	for _, digit := range ref[i:] {
		if digit < '0' || digit > '9' {
			return 0, 0, false
		}
	}
	row, err := strconv.Atoi(ref[i:])
	return row, column, err == nil && column > 0 && row > 0 && row <= 1_048_576
}
func (p *officePackage) editCell(x *officeXML, cell *officeNode, text string) ([]officePatch, error) {
	if len(cell.all(nsSheet, "f")) > 0 {
		return nil, ErrOfficeInvalidEdit
	}
	op, qname := officeOpening(x.data, cell)
	// Remove only the type attribute; keep styles, coordinates and other metadata.
	op = officeRemoveAttrs(op, "t")
	op = strings.TrimSuffix(strings.TrimSuffix(op, ">"), "/")
	prefix := ""
	if i := strings.IndexByte(qname, ':'); i >= 0 {
		prefix = qname[:i+1]
	}
	content := ""
	// Text beginning with '=' stays literal: basic editing never introduces formulas.
	if value, err := strconv.ParseFloat(text, 64); (cell.attr("t") == "" || cell.attr("t") == "n") && officeNumeric.MatchString(text) && err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) && (!strings.HasPrefix(text, "0") || text == "0" || strings.HasPrefix(text, "0.")) {
		content = "<" + prefix + "v>" + escapedOffice(text) + "</" + prefix + "v>"
	} else {
		op += ` t="inlineStr"`
		content = "<" + prefix + "is><" + prefix + `t xml:space="preserve">` + escapedOffice(text) + "</" + prefix + "t></" + prefix + "is>"
	}
	return []officePatch{{cell.start, cell.end, op + ">" + content + "</" + qname + ">"}}, nil
}
