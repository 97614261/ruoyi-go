package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/workflow"
)

func TestProductGraphOnlyExecutesRequestedImages(t *testing.T) {
	fields := model.PODProductFields{ProductName: "  马克杯 ", SKU: "internal-only", SellingPoints: "哑光\r\n350 mL", ProtectedDetails: "保留杯柄"}
	if err := productFields(&fields); err != nil {
		t.Fatal(err)
	}
	prompt := productPrompt(fields)
	if fields.ProductName != "马克杯" || !strings.Contains(prompt, "哑光\n350 mL") || strings.Contains(prompt, fields.SKU) {
		t.Fatalf("unexpected fields and prompt: %+v %q", fields, prompt)
	}
	graph := productGraph(model.PODProductCreate{ReferenceAssetID: "61", ModelID: "19", Ratio: "1:1"}, prompt, model.PODProductDefaultOutputs)
	for _, target := range []string{"", model.PODProductMain, model.PODProductScene} {
		t.Run(target, func(t *testing.T) {
			candidate := graph
			if target != "" {
				var err error
				candidate, err = workflow.ActionGraph(graph, "branch", target)
				if err != nil {
					t.Fatal(err)
				}
			}
			nodes, err := workflow.ExecutionPlanMode(candidate, model.PODGraphLimits{}, "", nil, "managed")
			if err != nil {
				t.Fatal(err)
			}
			var images []string
			for _, node := range nodes {
				if node.Type == "imageGenerate" {
					images = append(images, node.ID)
				}
			}
			want := []string{target}
			if target == "" {
				want = []string{model.PODProductMain, model.PODProductScene}
			}
			slices.Sort(images)
			if !slices.Equal(images, want) || len(nodes) != len(want)+2 {
				t.Fatalf("image calls %v, want %v with only two local inputs", images, want)
			}
			for _, id := range images {
				for _, port := range []string{"text", "images"} {
					if !slices.ContainsFunc(candidate.Edges, func(edge model.PODEdge) bool { return edge.Target == id && edge.TargetPort == port }) {
						t.Fatalf("missing %s input on %s", port, id)
					}
				}
			}
		})
	}
}

func TestProductZIPEntryMatchesArchivedBytes(t *testing.T) {
	data := []byte("archived image bytes")
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	var result bytes.Buffer
	writer := zip.NewWriter(&result)
	if err := addProductImage(context.Background(), writer, "main-white", "png", bytes.NewReader(data), int64(len(data)), hash); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(result.Bytes()), int64(result.Len()))
	if err != nil || len(reader.File) != 1 || reader.File[0].Name != "main-white.png" {
		t.Fatalf("unexpected archive: %+v %v", reader, err)
	}
	image, err := reader.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer image.Close()
	got, err := io.ReadAll(image)
	if !bytes.Equal(got, data) || err != nil {
		t.Fatalf("image bytes changed: %q %v", got, err)
	}
	writer = zip.NewWriter(io.Discard)
	if err := addProductImage(context.Background(), writer, "main-white", "png", strings.NewReader("not the archived file"), int64(len(data)), hash); err == nil {
		t.Fatal("accepted a file that does not match the stored checksum")
	}
}
