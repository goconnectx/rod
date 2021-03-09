package main

import (
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/go-rod/rod/lib/utils"
)

func main() {
	comment := `// This file is generated by "./lib/proto/generate"`

	schema := getSchema()

	init := comment + utils.S(`

		package proto

		import (
			"reflect"
			"github.com/ysmood/gson"
		)

		// Version of cdp protocol
		const Version = "v{{.major}}.{{.minor}}"

		var types = map[string]reflect.Type{
	`, "major", schema.Get("version.major").Str(), "minor", schema.Get("version.minor").Str())

	testsCode := comment + `

		package proto_test

		import (
			"github.com/go-rod/rod/lib/proto"
		)
	`

	for _, domain := range parse(schema) {

		code := comment + `

			package proto

			import (
				"github.com/ysmood/gson"
			)
		`

		code += fmt.Sprintf("/*\n\n%s\n\n", domain.name)

		if domain.description != "" {
			code += domain.description + "\n\n"
		}
		code += "*/\n\n"

		for _, definition := range domain.definitions {
			if definition.skip {
				continue
			}

			code += definition.format()
			testsCode += definition.formatTests()

			if definition.originName != "" {
				init += utils.S(`
					"{{.name}}": reflect.TypeOf({{.type}}{}),`,
					"name", definition.domain.name+"."+definition.originName,
					"type", definition.name,
				)
			}
		}

		utils.E(utils.OutputFile(
			filepath.FromSlash(
				fmt.Sprintf("lib/proto/%s.go", toSnakeCase(domain.name))),
			code))
	}

	init += `
		}
	`

	utils.E(utils.OutputFile(filepath.FromSlash("lib/proto/definitions.go"), init))
	utils.E(utils.OutputFile(filepath.FromSlash("lib/proto/definitions_test.go"), testsCode))

	path := "./lib/proto"
	utils.Exec("gofmt", "-s", "-w", path)
	utils.Exec(
		"go", "run", "github.com/ysmood/golangci-lint", "--",
		"run", "--no-config", "--fix", "--disable-all", "-E", "gofmt,goimports,misspell", path,
	)
}

func (d *definition) comment() string {
	comment := d.description

	if comment == "<nil>" {
		comment = "..."
	}

	if d.optional {
		comment = "(optional) " + comment
	}
	if d.experimental {
		comment = "(experimental) " + comment
	}
	if d.deprecated {
		comment = "(deprecated) " + comment
	}

	comment = symbol(d.name) + " " + comment

	return regexp.MustCompile(`(?m)^`).ReplaceAllString(comment, "// ")
}

func (d *definition) format() (code string) {
	switch d.objType {
	case objTypePrimitive:
		code = utils.S(`
		{{.comment}}
		type {{.name}} {{.type}}
		`, "name", d.name, "type", d.typeName, "comment", d.comment())

		if d.enum != nil {
			code += "const ("
			for _, value := range d.enum {
				name := d.name + symbol(value)
				code += utils.S(`
				// {{.name}} enum const
				{{.name}} {{.type}} = "{{.value}}"
				`, "name", name, "type", d.name, "value", value)
			}
			code += ")\n"
		}

	case objTypeStruct:
		code = utils.S(`
		{{.comment}}
		type {{.name}} struct {
		`, "name", d.name, "comment", d.comment())

		for _, prop := range d.props {
			tag := jsonTag(prop.originName, prop.optional)

			code += utils.S(`
			{{.comment}}
			{{.name}} {{.type}} {{.tag}}
			`, "comment", prop.comment(), "name", prop.name, "type", prop.typeName, "tag", tag)
		}

		code += "}\n"

		if d.command {
			method := d.domain.name + "." + d.originName
			if d.returnValue {
				code += utils.S(`
				// ProtoReq name
				func (m {{.name}}) ProtoReq() string { return "{{.method}}" }

				// Call the request
				func (m {{.name}}) Call(c Client) (*{{.name}}Result, error) {
					var res {{.name}}Result
					return &res, call(m.ProtoReq(), m, &res, c)
				}
				`, "name", d.name, "method", method)
			} else {
				code += utils.S(`
				// ProtoReq name
				func (m {{.name}}) ProtoReq() string { return "{{.method}}" }

				// Call sends the request
				func (m {{.name}}) Call(c Client) error {
					return call(m.ProtoReq(), m, nil, c)
				}
				`, "name", d.name, "method", method)
			}
		}

		if d.cdpType == cdpTypeEvents {
			code += utils.S(`
				// ProtoEvent name
				func (evt {{.name}}) ProtoEvent() string {
					return "{{.event}}"
				}
			`, "name", d.name, "event", d.domain.name+"."+d.originName)
		}
	}

	return
}

func (d *definition) formatTests() (code string) {
	switch d.cdpType {
	case cdpTypeCommands:
		if !d.command {
			return ""
		}

		if d.returnValue {
			return utils.S(`
				func (t T) {{.name}}() {
					c := &Client{}
					_, err := proto.{{.name}}{}.Call(c)
					t.Nil(err)
				}
				`, "name", d.name)
		}

		return utils.S(`
			func (t T) {{.name}}() {
				c := &Client{}
				err := proto.{{.name}}{}.Call(c)
				t.Nil(err)
			}
			`, "name", d.name)

	case cdpTypeEvents:
		return utils.S(`
		func (t T) {{.name}}() {
			e := proto.{{.name}}{}
			e.ProtoEvent()
		}
		`, "name", d.name)
	}

	return ""
}
