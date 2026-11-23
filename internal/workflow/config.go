package workflow

import (
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var fileNamePattern = regexp.MustCompile(`^[\p{L}\p{N}_ -]{1,80}$`)

func validateNodeConfig(kind string, config map[string]json.RawMessage) error {
	fields := map[string]string{"title": "title"}
	switch kind {
	case "textInput":
		fields["text"] = "text"
		fields["polishModelId"] = "modelId"
	case "imageInput":
		fields["assetIds"] = "assets"
	case "textGenerate":
		fields["instruction"] = "text"
		fields["modelId"] = "modelId"
	case "imageGenerate":
		fields["prompt"] = "text"
		fields["modelId"] = "modelId"
		fields["ratio"] = "ratio"
		fields["count"] = "count"
	case "imageProcess":
		fields["operation"] = "operation"
		fields["width"] = "size"
		fields["height"] = "size"
		fields["scale"] = "scale"
		fields["rotate"] = "rotate"
		fields["cropX"] = "position"
		fields["cropY"] = "position"
		fields["cropWidth"] = "fraction"
		fields["cropHeight"] = "fraction"
		fields["background"] = "color"
		fields["tolerance"] = "tolerance"
		fields["method"] = "method"
	case "productPreview":
		fields["product"] = "product"
		fields["color"] = "color"
		fields["printX"] = "position"
		fields["printY"] = "position"
		fields["printScale"] = "printScale"
		fields["side"] = "side"
	case "export":
		fields["format"] = "format"
		fields["fileName"] = "fileName"
		fields["mode"] = "mode"
		fields["printWidthMM"] = "millimetres"
		fields["printHeightMM"] = "millimetres"
	}
	for key, raw := range config {
		field, ok := fields[key]
		if !ok {
			return errors.New("unknown field")
		}
		if field == "assets" {
			var ids []string
			if json.Unmarshal(raw, &ids) != nil || ids == nil || len(ids) > 8 {
				return errors.New("invalid asset list")
			}
			seen := map[string]bool{}
			for _, id := range ids {
				n, err := strconv.ParseInt(id, 10, 64)
				if err != nil || n < 1 || strconv.FormatInt(n, 10) != id || seen[id] {
					return errors.New("invalid asset id")
				}
				seen[id] = true
			}
			continue
		}
		if in(field, "count", "size", "scale", "position", "printScale", "rotate", "fraction", "tolerance", "millimetres") {
			var number float64
			if string(raw) == "null" || json.Unmarshal(raw, &number) != nil {
				return errors.New("invalid number")
			}
			valid := false
			switch field {
			case "count":
				valid = number >= 1 && number <= 4 && math.Trunc(number) == number
			case "size":
				valid = number >= 64 && number <= 8192 && math.Trunc(number) == number
			case "scale":
				valid = number == 2 || number == 4
			case "position":
				valid = number >= 0 && number <= 1
			case "printScale":
				valid = number >= 0.05 && number <= 1.5
			case "rotate":
				valid = number == 0 || number == 90 || number == 180 || number == 270
			case "fraction":
				valid = number > 0 && number <= 1
			case "tolerance":
				valid = number >= 0 && number <= 120 && math.Trunc(number) == number
			case "millimetres":
				valid = number >= 1 && number <= 2000
			}
			if !valid {
				return errors.New("invalid number range")
			}
			continue
		}
		var value string
		if string(raw) == "null" || json.Unmarshal(raw, &value) != nil || strings.ContainsRune(value, 0) {
			return errors.New("invalid string")
		}
		valid := false
		switch field {
		case "modelId":
			if value == "" {
				valid = true
			} else {
				n, e := strconv.ParseInt(value, 10, 64)
				valid = e == nil && n > 0 && strconv.FormatInt(n, 10) == value
			}
		case "title":
			valid = utf8.RuneCountInString(value) <= 60
		case "text":
			valid = utf8.RuneCountInString(value) <= 12000
		case "ratio":
			valid = in(value, "1:1", "3:4", "4:3", "9:16", "16:9")
		case "operation":
			valid = in(value, "resize", "crop", "removeBackground", "upscale")
		case "product":
			valid = value == "tshirt"
		case "color":
			valid = colorPattern.MatchString(value)
		case "format":
			valid = in(value, "png", "jpeg", "webp")
		case "fileName":
			valid = fileNamePattern.MatchString(value)
		case "mode":
			valid = in(value, "print", "mockup", "image")
		case "method":
			valid = in(value, "basic", "ai")
		case "side":
			valid = in(value, "front", "back")
		}
		if !valid {
			return errors.New("invalid value")
		}
	}
	return nil
}
func in(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}
