// Package parser implements parsing of Terraform modules to extract metadata.
package parser

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cloudnationhq/az-cn-wam-mcp/internal/util"
	"github.com/cloudnationhq/az-cn-wam-mcp/pkg/terraform"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

type TerraformParser struct {
	parser  *hclparse.Parser
	learner *CategoryLearner
}

func NewTerraformParser() *TerraformParser {
	return &TerraformParser{
		parser:  hclparse.NewParser(),
		learner: NewCategoryLearner(),
	}
}

func (p *TerraformParser) SetLearner(learner *CategoryLearner) {
	p.learner = learner
}

func (p *TerraformParser) ParseModule(modulePath string) (*terraform.Module, error) {
	module := &terraform.Module{
		Path:      modulePath,
		Name:      extractModuleName(modulePath),
		Variables: []terraform.Variable{},
		Outputs:   []terraform.Output{},
		Resources: []terraform.Resource{},
		Examples:  []terraform.Example{},
	}

	err := filepath.WalkDir(modulePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !strings.HasSuffix(path, ".tf") {
			return nil
		}

		if strings.Contains(path, "examples/") {
			return nil
		}

		return p.parseFile(path, module)
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse module %s: %w", modulePath, err)
	}

	if err := p.parseExamples(modulePath, module); err != nil {
		fmt.Printf("Warning: failed to parse examples for %s: %v\n", modulePath, err)
	}

	if desc := p.extractDescription(modulePath); desc != "" {
		module.Description = desc
	}

	module.Provider = p.detectProvider(modulePath, module.Resources)
	module.Tags = p.categorizeModule(module)

	return module, nil
}

func (p *TerraformParser) parseFile(filePath string, module *terraform.Module) error {
	src, err := readFile(filePath)
	if err != nil {
		return err
	}

	file, diags := p.parser.ParseHCL(src, filePath)
	if diags.HasErrors() {
		return fmt.Errorf("failed to parse %s: %s", filePath, diags.Error())
	}

	body := file.Body.(*hclsyntax.Body)

	for _, block := range body.Blocks {
		switch block.Type {
		case "variable":
			if len(block.Labels) > 0 {
				variable := p.parseVariable(block)
				module.Variables = append(module.Variables, variable)
			}
		case "output":
			if len(block.Labels) > 0 {
				output := p.parseOutput(block)
				module.Outputs = append(module.Outputs, output)
			}
		case "resource":
			if len(block.Labels) >= 2 {
				resource := p.parseResource(block)
				module.Resources = append(module.Resources, resource)
			}
		}
	}

	return nil
}

func (p *TerraformParser) parseVariable(block *hclsyntax.Block) terraform.Variable {
	variable := terraform.Variable{
		Name:     block.Labels[0],
		Required: true,
	}

	if desc := p.getAttributeValue(block, "description"); desc != "" {
		variable.Description = desc
	}

	if typeExpr := p.getAttributeValue(block, "type"); typeExpr != "" {
		variable.Type = typeExpr
	}

	if p.hasAttribute(block, "default") {
		variable.Required = false
		variable.Default = p.getDefaultValue(block, "default")
	}

	if p.hasAttribute(block, "sensitive") {
		variable.Sensitive = true
	}

	return variable
}

func (p *TerraformParser) parseOutput(block *hclsyntax.Block) terraform.Output {
	output := terraform.Output{
		Name: block.Labels[0],
	}

	if desc := p.getAttributeValue(block, "description"); desc != "" {
		output.Description = desc
	}

	if p.hasAttribute(block, "sensitive") {
		output.Sensitive = true
	}

	return output
}

func (p *TerraformParser) parseResource(block *hclsyntax.Block) terraform.Resource {
	return terraform.Resource{
		Type:     block.Labels[0],
		Name:     block.Labels[1],
		Provider: extractProvider(block.Labels[0]),
	}
}

func (p *TerraformParser) parseExamples(modulePath string, module *terraform.Module) error {
	examplesPath := filepath.Join(modulePath, "examples")

	return filepath.WalkDir(examplesPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() && path != examplesPath {
			exampleName := d.Name()
			example := terraform.Example{
				Name: exampleName,
				Path: path,
			}

			mainTf := filepath.Join(path, "main.tf")
			if content, err := readFile(mainTf); err == nil {
				example.Content = string(content)
			}

			module.Examples = append(module.Examples, example)
		}

		return nil
	})
}

func (p *TerraformParser) getAttributeValue(block *hclsyntax.Block, name string) string {
	if attr, exists := block.Body.Attributes[name]; exists {
		if expr, ok := attr.Expr.(*hclsyntax.LiteralValueExpr); ok {
			return expr.Val.AsString()
		}
	}
	return ""
}

func (p *TerraformParser) hasAttribute(block *hclsyntax.Block, name string) bool {
	_, exists := block.Body.Attributes[name]
	return exists
}

func (p *TerraformParser) getDefaultValue(block *hclsyntax.Block, name string) any {
	attr, exists := block.Body.Attributes[name]
	if !exists {
		return nil
	}

	val, err := p.extractCtyValue(attr.Expr)
	if err != nil {
		return nil
	}

	return p.ctyValueToGo(val)
}

func (p *TerraformParser) extractCtyValue(expr hclsyntax.Expression) (cty.Value, error) {
	switch e := expr.(type) {
	case *hclsyntax.LiteralValueExpr:
		return e.Val, nil
	case *hclsyntax.TupleConsExpr:
		values := make([]cty.Value, len(e.Exprs))
		for i, expr := range e.Exprs {
			val, err := p.extractCtyValue(expr)
			if err != nil {
				return cty.NilVal, err
			}
			values[i] = val
		}
		if len(values) == 0 {
			return cty.ListValEmpty(cty.DynamicPseudoType), nil
		}
		return cty.TupleVal(values), nil
	case *hclsyntax.ObjectConsExpr:
		values := make(map[string]cty.Value)
		for _, item := range e.Items {
			keyExpr, ok := item.KeyExpr.(*hclsyntax.ObjectConsKeyExpr)
			if !ok {
				continue
			}
			key := ""
			if wrapped, ok := keyExpr.Wrapped.(*hclsyntax.ScopeTraversalExpr); ok {
				key = wrapped.Traversal.RootName()
			}
			if key == "" {
				continue
			}
			val, err := p.extractCtyValue(item.ValueExpr)
			if err != nil {
				return cty.NilVal, err
			}
			values[key] = val
		}
		if len(values) == 0 {
			return cty.EmptyObjectVal, nil
		}
		return cty.ObjectVal(values), nil
	default:
		return cty.NullVal(cty.DynamicPseudoType), nil
	}
}

func (p *TerraformParser) ctyValueToGo(val cty.Value) any {
	if val.IsNull() {
		return nil
	}

	valType := val.Type()

	switch {
	case valType == cty.String:
		return val.AsString()
	case valType == cty.Number:
		var f float64
		if err := gocty.FromCtyValue(val, &f); err == nil {
			if f == float64(int64(f)) {
				return int64(f)
			}
			return f
		}
		return nil
	case valType == cty.Bool:
		return val.True()
	case valType.IsListType() || valType.IsTupleType():
		var result []any
		it := val.ElementIterator()
		for it.Next() {
			_, elemVal := it.Element()
			result = append(result, p.ctyValueToGo(elemVal))
		}
		return result
	case valType.IsMapType() || valType.IsObjectType():
		result := make(map[string]any)
		it := val.ElementIterator()
		for it.Next() {
			key, elemVal := it.Element()
			keyStr := key.AsString()
			result[keyStr] = p.ctyValueToGo(elemVal)
		}
		return result
	case valType.IsSetType():
		var result []any
		it := val.ElementIterator()
		for it.Next() {
			_, elemVal := it.Element()
			result = append(result, p.ctyValueToGo(elemVal))
		}
		return result
	default:
		return nil
	}
}

func extractModuleName(path string) string {
	return filepath.Base(path)
}

func extractProvider(resourceType string) string {
	return util.ExtractProvider(resourceType)
}

func (p *TerraformParser) extractDescription(modulePath string) string {
	readmePath := filepath.Join(modulePath, "README.md")
	if content, err := readFile(readmePath); err == nil {
		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			if strings.HasPrefix(line, "#") && i+1 < len(lines) {
				if desc := strings.TrimSpace(lines[i+1]); desc != "" && !strings.HasPrefix(desc, "#") {
					return desc
				}
			}
		}
	}
	return ""
}

type CategoryLearner struct {
	resourceTypes    map[string]int
	resourceClusters map[string][]string
	textPatterns     map[string]map[string]int
}

func NewCategoryLearner() *CategoryLearner {
	return &CategoryLearner{
		resourceTypes:    make(map[string]int),
		resourceClusters: make(map[string][]string),
		textPatterns:     make(map[string]map[string]int),
	}
}

func (cl *CategoryLearner) LearnFromModule(module *terraform.Module) {
	moduleResources := []string{}
	for _, resource := range module.Resources {
		cl.resourceTypes[resource.Type]++
		moduleResources = append(moduleResources, resource.Type)
	}

	if len(moduleResources) > 1 {
		key := strings.Join(moduleResources, ",")
		cl.resourceClusters[key] = moduleResources
	}

	text := strings.ToLower(module.Name + " " + module.Description)
	words := strings.Fields(text)

	categoryHint := extractCategoryHint(module.Name)
	if categoryHint != "" {
		if cl.textPatterns[categoryHint] == nil {
			cl.textPatterns[categoryHint] = make(map[string]int)
		}
		for _, word := range words {
			if len(word) > 3 { // Skip short words
				cl.textPatterns[categoryHint][word]++
			}
		}
	}
}

func (cl *CategoryLearner) GetLearnedCategories(resourceType string) []string {
	categories := []string{}

	for _, resources := range cl.resourceClusters {
		if slices.Contains(resources, resourceType) {
			category := cl.deriveClusterCategory(resources)
			if category != "" {
				categories = append(categories, category)
			}
		}
	}

	if len(categories) == 0 {
		if category := cl.deriveResourceCategory(resourceType); category != "" {
			categories = append(categories, category)
		}
	}

	return categories
}

func (cl *CategoryLearner) GetLearnedTextCategories(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	categoryScores := make(map[string]int)

	for category, wordCounts := range cl.textPatterns {
		score := 0
		for _, word := range words {
			if count, exists := wordCounts[word]; exists {
				score += count
			}
		}
		if score > 0 {
			categoryScores[category] = score
		}
	}

	var categories []string
	for category := range categoryScores {
		categories = append(categories, category)
	}

	return categories
}

func (p *TerraformParser) categorizeModule(module *terraform.Module) []string {
	categories := []string{}
	categoryMap := make(map[string]bool)

	if p.learner != nil {
		for _, resource := range module.Resources {
			cats := p.learner.GetLearnedCategories(resource.Type)
			for _, cat := range cats {
				if !categoryMap[cat] {
					categories = append(categories, cat)
					categoryMap[cat] = true
				}
			}
		}

		text := module.Name + " " + module.Description
		textCats := p.learner.GetLearnedTextCategories(text)
		for _, cat := range textCats {
			if !categoryMap[cat] {
				categories = append(categories, cat)
				categoryMap[cat] = true
			}
		}
	} else {
		categories = p.extractDirectCategories(module)
	}

	if module.Provider != "" && !categoryMap[module.Provider] {
		categories = append(categories, module.Provider)
	}

	return categories
}

func (p *TerraformParser) extractDirectCategories(module *terraform.Module) []string {
	categoryMap := make(map[string]bool)

	for _, resource := range module.Resources {
		parts := strings.Split(resource.Type, "_")
		if len(parts) > 1 {
			if len(parts[1]) > 3 {
				categoryMap[parts[1]] = true
			}
			if len(parts) > 2 && len(parts[len(parts)-1]) > 3 {
				categoryMap[parts[len(parts)-1]] = true
			}
		}
	}

	nameParts := strings.Split(strings.ToLower(module.Name), "-")
	for _, part := range nameParts {
		if len(part) > 3 && part != "terraform" && part != "azure" {
			categoryMap[part] = true
		}
	}

	var categories []string
	for cat := range categoryMap {
		categories = append(categories, cat)
	}

	return categories
}

func (cl *CategoryLearner) deriveClusterCategory(resources []string) string {
	wordCount := make(map[string]int)
	for _, rt := range resources {
		parts := strings.Split(rt, "_")
		for _, part := range parts {
			if len(part) > 3 && part != "azurerm" {
				wordCount[part]++
			}
		}
	}

	maxCount := 0
	category := ""
	for word, count := range wordCount {
		if count > maxCount {
			maxCount = count
			category = word
		}
	}

	return category
}

func (cl *CategoryLearner) deriveResourceCategory(resourceType string) string {
	parts := strings.Split(resourceType, "_")
	if len(parts) > 1 {
		for i := 1; i < len(parts); i++ {
			if len(parts[i]) > 3 {
				return parts[i]
			}
		}
	}
	return ""
}

func extractCategoryHint(moduleName string) string {
	parts := strings.Split(strings.ToLower(moduleName), "-")
	for _, part := range parts {
		if len(part) > 3 && part != "terraform" && part != "azure" {
			return part
		}
	}
	return ""
}

func (p *TerraformParser) detectProvider(modulePath string, resources []terraform.Resource) string {
	terraformFiles := []string{"terraform.tf", "versions.tf", "providers.tf", "main.tf"}

	for _, filename := range terraformFiles {
		filePath := filepath.Join(modulePath, filename)
		if content, err := os.ReadFile(filePath); err == nil {
			if provider := p.extractProviderFromContent(string(content)); provider != "" {
				return provider
			}
		}
	}

	providerMap := make(map[string]int)
	for _, resource := range resources {
		provider := extractProvider(resource.Type)
		providerMap[provider]++
	}

	maxCount := 0
	primaryProvider := "unknown"
	for provider, count := range providerMap {
		if count > maxCount {
			maxCount = count
			primaryProvider = provider
		}
	}

	return primaryProvider
}

func (p *TerraformParser) extractProviderFromContent(content string) string {
	file, diags := p.parser.ParseHCL([]byte(content), "temp.tf")
	if diags.HasErrors() {
		return ""
	}

	body := file.Body.(*hclsyntax.Body)
	for _, block := range body.Blocks {
		if block.Type == "terraform" {
			for _, innerBlock := range block.Body.Blocks {
				if innerBlock.Type == "required_providers" {
					for name := range innerBlock.Body.Attributes {
						return name
					}
				}
			}
		} else if block.Type == "provider" && len(block.Labels) > 0 {
			return block.Labels[0]
		}
	}

	return ""
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
