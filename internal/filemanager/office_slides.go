package filemanager

import (
	"math"
	"strconv"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func (p *officePackage) slides() ([]contract.OfficeSection, error) {
	x, err := p.part("ppt/presentation.xml")
	if err != nil {
		return nil, err
	}
	if !x.root.is(nsSlide, "presentation") {
		return nil, ErrOfficeUnsupported
	}
	rels, err := p.relationships("ppt/presentation.xml")
	if err != nil {
		return nil, err
	}
	width, height := 960.0, 540.0
	if size := x.root.child(nsSlide, "sldSz"); size != nil {
		width = max(1, officeNumber(size, "cx", 960))
		height = max(1, officeNumber(size, "cy", 540))
	}
	var sections []contract.OfficeSection
	seenParts := map[string]bool{}
	for _, slide := range x.root.all(nsSlide, "sldId") {
		if len(sections) >= 128 {
			return nil, ErrTooLarge
		}
		part := rels[slide.relID()]
		if part == "" || seenParts[part] {
			return nil, ErrOfficeUnsupported
		}
		seenParts[part] = true
		s, err := p.part(part)
		if err != nil {
			return nil, err
		}
		if !s.root.is(nsSlide, "sld") {
			return nil, ErrOfficeUnsupported
		}
		images, err := p.relationships(part)
		if err != nil {
			return nil, err
		}
		section := contract.OfficeSection{Name: strconv.Itoa(len(sections) + 1), Width: width, Height: height, Items: []contract.OfficeItem{}}
		shapes := s.root.child(nsSlide, "cSld").child(nsSlide, "spTree")
		if shapes == nil {
			return nil, ErrOfficeUnsupported
		}
		for _, shape := range shapes.children {
			if !shape.is(nsSlide, "sp") && !shape.is(nsSlide, "pic") {
				continue
			}
			xpos, ypos, w, h := 0.0, 0.0, width, height
			if transform := shape.child(nsSlide, "spPr").child(nsDraw, "xfrm"); transform != nil {
				if offset := transform.child(nsDraw, "off"); offset != nil {
					xpos = officeNumber(offset, "x", 0)
					ypos = officeNumber(offset, "y", 0)
				}
				if extent := transform.child(nsDraw, "ext"); extent != nil {
					w = officeNumber(extent, "cx", width)
					h = officeNumber(extent, "cy", height)
				}
			}
			if shape.is(nsSlide, "pic") {
				for _, blip := range shape.all(nsDraw, "blip") {
					if img := p.imageItem(images[officeEmbed(blip)]); img != nil {
						img.X, img.Y, img.Width, img.Height = xpos, ypos, w, h
						section.Items = append(section.Items, *img)
					}
				}
				continue
			}
			paras := shape.child(nsSlide, "txBody").all(nsDraw, "p")
			for index, para := range paras {
				item, err := p.textItem(part, para, nsDraw)
				if err != nil {
					return nil, err
				}
				item.X, item.Y, item.Width, item.Height = xpos, ypos+h*float64(index)/float64(len(paras)), w, h/float64(len(paras))
				if props := para.child(nsDraw, "pPr"); props != nil {
					item.Align = props.attr("algn")
				}
				for _, run := range para.children {
					if run.is(nsDraw, "r") {
						if props := run.child(nsDraw, "rPr"); props != nil {
							item.FontSize = officeNumber(props, "sz", 1800) / 100
							item.Bold = props.attr("b") == "1"
							item.Italic = props.attr("i") == "1"
						}
						break
					}
				}
				section.Items = append(section.Items, item)
			}
		}
		sections = append(sections, section)
	}
	if len(sections) == 0 {
		return nil, ErrOfficeUnsupported
	}
	return sections, nil
}
func officeNumber(n *officeNode, key string, fallback float64) float64 {
	value, err := strconv.ParseFloat(n.attr(key), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1e10 {
		return fallback
	}
	return value
}
