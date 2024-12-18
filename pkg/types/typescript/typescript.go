package typescript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sst/sst/internal/fs"
	"github.com/sst/sst/pkg/js"
	"github.com/sst/sst/pkg/project/common"
)

var mapping = map[string]string{
	"r2BucketBindings":    "R2Bucket",
	"d1DatabaseBindings":  "D1Database",
	"kvNamespaceBindings": "KVNamespace",
	"queueBindings":       "Queue",
	"serviceBindings":     "Service",
}

func Generate(root string, links common.Links) error {
	cloudflareBindings := map[string]string{}
	for name, link := range links {
		for _, include := range link.Include {
			if include.Type == "cloudflare.binding" {
				binding := include.Other["binding"].(string)
				cloudflareBindings[name] = mapping[binding]
				break
			}
		}
	}

	header := []byte(strings.Join([]string{
		"/* This file is auto-generated by SST. Do not edit. */",
		"/* tslint:disable */",
		"/* eslint-disable */",
		"/* deno-fmt-ignore-file */",
		"import \"sst\"",
		"export {}",
		"",
	}, "\n"))
	packageJsons := fs.FindDown(root, "package.json")
	for _, packageJson := range packageJsons {
		packageJsonFile, err := os.Open(packageJson)
		if err != nil {
			continue
		}
		var data js.PackageJson
		err = json.NewDecoder(packageJsonFile).Decode(&data)
		if err != nil {
			continue
		}
		envPath := filepath.Join(filepath.Dir(packageJson), "sst-env.d.ts")
		envFile, err := os.Create(envPath)
		if err != nil {
			continue
		}
		defer envFile.Close()
		envFile.Write(header)

		properties := map[string]interface{}{}
		for name, link := range links {
			properties[name] = link.Properties
		}

		if data.Dependencies["@cloudflare/workers-types"] != "" || data.DevDependencies["@cloudflare/workers-types"] != "" {
			nonCloudflareLinks := map[string]interface{}{}
			for name, link := range properties {
				if cloudflareBindings[name] == "" {
					nonCloudflareLinks[name] = link
				}
			}
			envFile.WriteString("import \"sst\"\n")
			envFile.WriteString("declare module \"sst\" {\n")
			envFile.WriteString("  export interface Resource " + infer(nonCloudflareLinks, "  ") + "\n")
			envFile.WriteString("}" + "\n")
			bindings := map[string]interface{}{}
			for name, link := range cloudflareBindings {
				bindings[name] = literal{value: `cloudflare.` + link}
			}
			if len(bindings) > 0 {
				envFile.WriteString("// cloudflare \n")
				envFile.WriteString("import * as cloudflare from \"@cloudflare/workers-types\";\n")
				envFile.WriteString("declare module \"sst\" {\n")
				envFile.WriteString("  export interface Resource " + infer(bindings, "  ") + "\n")
				envFile.WriteString("}\n")
			}
			continue
		}
		envFile.Write([]byte("declare module \"sst\" {\n"))
		envFile.Write([]byte("  export interface Resource " + infer(properties, "  ") + "\n"))
		envFile.Write([]byte("}\n"))
	}

	return nil
}

type literal struct {
	value string
}

func infer(input map[string]interface{}, indentArgs ...string) string {
	indent := ""
	if len(indentArgs) > 0 {
		indent = indentArgs[0]
	}
	var builder strings.Builder
	builder.WriteString("{")
	builder.WriteString("\n")
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := input[key]
		builder.WriteString(indent + "  \"" + key + "\": ")
		if key == "type" && len(indentArgs) == 1 {
			builder.WriteString("\"")
			builder.WriteString(value.(string))
			builder.WriteString("\"")
		} else {
			switch v := value.(type) {
			case literal:
				builder.WriteString(v.value)
			case string:
				builder.WriteString("string")
			case int:
				builder.WriteString("number")
			case float64:
				builder.WriteString("number")
			case float32:
				builder.WriteString("number")
			case bool:
				builder.WriteString("boolean")
			case map[string]interface{}:
				builder.WriteString(infer(value.(map[string]interface{}), indent+"  "))
			default:
				builder.WriteString("any")
			}
		}
		builder.WriteString("\n")
	}
	builder.WriteString(indent + "}")
	return builder.String()
}
