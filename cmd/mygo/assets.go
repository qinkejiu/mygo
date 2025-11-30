package main

import (
	"bytes"
	_ "embed"
	"text/template"
)

var (
	//go:embed templates/verilator_driver.cpp.tmpl
	rawVerilatorDriverTemplate string

	//go:embed templates/xargs_shim.py
	xargsShimScript string

	verilatorDriverTemplate = template.Must(template.New("verilator_driver").Parse(rawVerilatorDriverTemplate))
)

type verilatorDriverData struct {
	MaxCycles   int
	ResetCycles int
}

func renderVerilatorDriver(maxCycles, resetCycles int) (string, error) {
	var buf bytes.Buffer
	data := verilatorDriverData{
		MaxCycles:   maxCycles,
		ResetCycles: resetCycles,
	}
	if err := verilatorDriverTemplate.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
