package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/speedata/goxml"
	"github.com/speedata/goxpath"
	"github.com/speedata/goxslt"
	"github.com/speedata/optionparser"
)

func main() {
	var sourcePath, xslPath, outputPath string
	op := optionparser.NewOptionParser()
	op.Banner = "Usage: goxslt -s source.xml -t stylesheet.xsl [-o output.xml] [param=value ...]"
	op.On("-s", "--source FILE", "Source XML file", &sourcePath)
	op.On("-t", "--xsl FILE", "XSLT stylesheet file", &xslPath)
	op.On("-o", "--output FILE", "Write output to FILE (default: stdout)", &outputPath)
	err := op.Parse()
	if errors.Is(err, optionparser.ErrHelp) {
		os.Exit(0)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if sourcePath == "" || xslPath == "" {
		fmt.Fprintln(os.Stderr, "Error: both --source and --xsl are required")
		op.Help()
		os.Exit(1)
	}

	// Parse remaining arguments as stylesheet parameters (foo=bar).
	params := make(map[string]goxpath.Sequence)
	for _, arg := range op.Extra {
		key, value, ok := strings.Cut(arg, "=")
		if !ok {
			fmt.Fprintf(os.Stderr, "Error: invalid parameter %q (expected key=value)\n", arg)
			os.Exit(1)
		}
		params[key] = goxpath.Sequence{value}
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening source: %v\n", err)
		os.Exit(1)
	}
	defer sourceFile.Close()

	sourceDoc, err := goxml.Parse(sourceFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing source XML: %v\n", err)
		os.Exit(1)
	}

	ss, err := goxslt.CompileFile(xslPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error compiling stylesheet: %v\n", err)
		os.Exit(1)
	}

	opts := goxslt.TransformOptions{}
	if len(params) > 0 {
		opts.Parameters = params
	}

	transformResult, err := goxslt.TransformWithOptions(ss, sourceDoc, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error transforming: %v\n", err)
		os.Exit(1)
	}

	result := goxslt.SerializeWithOutput(transformResult.Document, transformResult.Output)

	if outputPath != "" {
		if err := os.WriteFile(outputPath, []byte(result), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing output file: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Print(result)
	}

	// Write secondary documents (xsl:result-document).
	for href, doc := range transformResult.SecondaryDocuments {
		secResult := goxslt.SerializeWithOutput(doc, transformResult.Output)
		secPath := href
		if outputPath != "" {
			secPath = filepath.Join(filepath.Dir(outputPath), href)
		}
		if err := os.MkdirAll(filepath.Dir(secPath), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory for %s: %v\n", secPath, err)
			os.Exit(1)
		}
		if err := os.WriteFile(secPath, []byte(secResult), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing secondary document %s: %v\n", secPath, err)
			os.Exit(1)
		}
	}
}
