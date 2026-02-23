/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package secretbackend

import (
	"bytes"
	"fmt"
	"text/template"
)

// PathBuilder builds secret paths from templates
type PathBuilder struct {
	template *template.Template
}

// PathVariables holds the variables for path template expansion
type PathVariables struct {
	Region   string
	Hostname string
	Username string
}

// NewPathBuilder creates a new PathBuilder with the given template string
func NewPathBuilder(templateStr string) (*PathBuilder, error) {
	tmpl, err := template.New("path").Parse(templateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse path template: %w", err)
	}

	return &PathBuilder{
		template: tmpl,
	}, nil
}

// Build constructs a path using the provided variables
func (pb *PathBuilder) Build(vars PathVariables) (string, error) {
	var buf bytes.Buffer
	if err := pb.template.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("failed to execute path template: %w", err)
	}
	return buf.String(), nil
}
