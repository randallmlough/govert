package govert

import (
	"fmt"
	"go/types"
	"io/ioutil"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/99designs/gqlgen/codegen/config"
	"github.com/99designs/gqlgen/codegen/templates"
	"github.com/iancoleman/strcase"
	. "github.com/logrusorgru/aurora"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/web-ridge/go-pluralize"
)

var pathRegex *regexp.Regexp
var pluralizer *pluralize.Client

func init() {
	var initError error
	pluralizer = pluralize.NewClient()
	pathRegex, initError = regexp.Compile(`src\/(.*)`)
	if initError != nil {
		fmt.Println("could not compile the path regex")
	}
}

type Convert struct {
	output         Directory
	backend        Directory
	frontend       Directory
	rootImportPath string
	primaryKeyType reflect.Type
}

type Directory struct {
	Directory string
	Package   string
}

func (m *Convert) Name() string {
	return "convert-generator"
}

func (m *Convert) MutateConfig(originalCfg *config.Config) error {
	t := &Template{
		PackageName: m.output.Package,
		Backend: Directory{
			Directory: path.Join(m.rootImportPath, m.backend.Directory),
			Package:   m.backend.Package,
		},
		Frontend: Directory{
			Directory: path.Join(m.rootImportPath, m.frontend.Directory),
			Package:   m.frontend.Package,
		},
		PrimaryKey: m.primaryKeyType,
	}

	cfg := copyConfig(*originalCfg)

	fmt.Println(BrightGreen("[convert]"), " get boiler models")
	boilerModels := GetBoilerModels(m.backend.Directory)

	fmt.Println(BrightGreen("[convert]"), " get extra's from schema")
	interfaces, enums, scalars := getExtrasFromSchema(cfg.Schema)

	fmt.Println(BrightGreen("[convert]"), " get model with information")
	models := GetModelsWithInformation(enums, originalCfg, boilerModels)

	t.Models = models
	t.HasStringPrimaryIDs = HasStringPrimaryIDsInModels(models)
	t.Interfaces = interfaces
	t.Enums = enums
	t.Scalars = scalars
	if len(t.Models) == 0 {
		fmt.Println(Red("No models found in graphql so skipping generation").Bold())
		return nil
	}

	fmt.Println(BrightGreen("[convert]"), " render preload.gotpl")
	templates.CurrentImports = nil
	if renderError := m.generatePreloadFile(cfg, t); renderError != nil {
		fmt.Println(BrightRed("renderError"), renderError)
	}
	templates.CurrentImports = nil
	fmt.Println(BrightGreen("[convert]"), " render convert.gotpl")
	if renderError := m.generateConvertFile(cfg, t); renderError != nil {
		fmt.Println(BrightRed("renderError"), renderError)
	}

	templates.CurrentImports = nil
	fmt.Println(BrightGreen("[convert]"), " render convert_input.gotpl")
	if renderError := m.generateConvertInputFile(cfg, t); renderError != nil {
		fmt.Println(BrightRed("renderError"), renderError)
	}

	templates.CurrentImports = nil
	fmt.Println(BrightGreen("[convert]"), " render filter.gotpl")
	if renderError := m.generateFilterFile(cfg, t); renderError != nil {
		fmt.Println(BrightRed("renderError"), renderError)
	}

	templates.CurrentImports = nil
	fmt.Println(BrightGreen("[convert]"), " generating ID file")
	if renderError := m.generateIDFile(cfg, t); renderError != nil {
		fmt.Println(BrightRed("renderError"), renderError)
	}
	return nil
}
func (c *Convert) generatePreloadFile(cfg *config.Config, data *Template) error {
	if err := templates.Render(templates.Options{
		Template:        getTemplate("preload.gotpl"),
		PackageName:     c.output.Package,
		Filename:        c.output.Directory + "/" + "preload.go",
		Data:            data,
		GeneratedHeader: true,
		Packages:        cfg.Packages,
	}); err != nil {
		return fmt.Errorf("failed to render ID file %w", err)
	}
	return nil
}
func (c *Convert) generateConvertFile(cfg *config.Config, data *Template) error {
	if err := templates.Render(templates.Options{
		Template:        getTemplate("convert.gotpl"),
		PackageName:     c.output.Package,
		Filename:        c.output.Directory + "/" + "convert.go",
		Data:            data,
		GeneratedHeader: true,
		Packages:        cfg.Packages,
	}); err != nil {
		return fmt.Errorf("failed to render ID file %w", err)
	}
	return nil
}
func (c *Convert) generateConvertInputFile(cfg *config.Config, data *Template) error {
	if err := templates.Render(templates.Options{
		Template:        getTemplate("convert_input.gotpl"),
		PackageName:     c.output.Package,
		Filename:        c.output.Directory + "/" + "convert_input.go",
		Data:            data,
		GeneratedHeader: true,
		Packages:        cfg.Packages,
	}); err != nil {
		return fmt.Errorf("failed to render ID file %w", err)
	}
	return nil
}
func (c *Convert) generateFilterFile(cfg *config.Config, data *Template) error {
	if err := templates.Render(templates.Options{
		Template:        getTemplate("filter.gotpl"),
		PackageName:     c.output.Package,
		Filename:        c.output.Directory + "/" + "filter.go",
		Data:            data,
		GeneratedHeader: true,
		Packages:        cfg.Packages,
	}); err != nil {
		return fmt.Errorf("failed to generate filter file %w", err)
	}
	return nil
}

func (c *Convert) generateIDFile(cfg *config.Config, data *Template) error {
	if err := templates.Render(templates.Options{
		Template:        getTemplate("id.gotpl"),
		PackageName:     c.output.Package,
		Filename:        c.output.Directory + "/" + "id.go",
		Data:            data,
		GeneratedHeader: true,
		Funcs:           funcMap,
		Packages:        cfg.Packages,
	}); err != nil {
		return fmt.Errorf("failed to generate ID file %w", err)
	}
	return nil
}

type Template struct {
	Backend             Directory
	Frontend            Directory
	HasStringPrimaryIDs bool
	PackageName         string
	Interfaces          []*Interface
	Models              []*Model
	Enums               []*Enum
	Scalars             []string
	PrimaryKey          reflect.Type
}

type Interface struct {
	Description string
	Name        string
}

type Preload struct {
	Key           string
	ColumnSetting ColumnSetting
}

type Model struct {
	Name                  string
	PluralName            string
	BoilerModel           BoilerModel
	PrimaryKeyType        string
	Fields                []*Field
	IsNormal              bool
	IsInput               bool
	IsCreateInput         bool
	IsUpdateInput         bool
	IsNormalInput         bool
	IsPayload             bool
	IsWhere               bool
	IsFilter              bool
	IsPreloadable         bool
	PreloadArray          []Preload
	HasOrganizationID     bool
	HasUserOrganizationID bool
	HasUserID             bool
	HasStringPrimaryID    bool
	// other stuff
	Description string
	PureFields  []*ast.FieldDefinition
	Implements  []string
}

type ColumnSetting struct {
	Name                  string
	RelationshipModelName string
	IDAvailable           bool
}

type Field struct {
	Name               string
	PluralName         string
	Type               string
	TypeWithoutPointer string
	IsNumberID         bool
	IsPrimaryNumberID  bool
	IsPrimaryID        bool
	IsRequired         bool
	IsPlural           bool
	ConvertConfig      ConvertConfig
	// relation stuff
	IsRelation bool
	// boiler relation stuff is inside this field
	BoilerField BoilerField
	// graphql relation ship can be found here
	Relationship *Model
	IsOr         bool
	IsAnd        bool

	// Some stuff
	Description  string
	OriginalType types.Type
	Tag          string
}

type Enum struct {
	Description string
	Name        string

	Values []*EnumValue
}

type EnumValue struct {
	Description string
	Name        string
	NameLower   string
}

func GetModelsWithInformation(enums []*Enum, cfg *config.Config, boilerModels []*BoilerModel) []*Model {

	// get models based on the schema and sqlboiler structs
	models := getModelsFromSchema(cfg.Schema, boilerModels)

	// Now we have all model's let enhance them with fields
	enhanceModelsWithFields(enums, cfg.Schema, cfg, models)

	// Add preload maps
	enhanceModelsWithPreloadArray(models)

	// Sort in same order
	sort.Slice(models, func(i, j int) bool { return models[i].Name < models[j].Name })
	for _, m := range models {
		cfg.Models.Add(m.Name, cfg.Model.ImportPath()+"."+templates.ToGo(m.Name))
	}
	return models
}

func getTemplate(filename string) string {
	// load path relative to calling source file
	_, callerFile, _, _ := runtime.Caller(1)
	rootDir := filepath.Dir(callerFile)
	content, err := ioutil.ReadFile(path.Join(rootDir, filename))
	if err != nil {
		fmt.Println("Could not read .gotpl file", err)
		return "Could not read .gotpl file"
	}
	return string(content)
}
func HasStringPrimaryIDsInModels(models []*Model) bool {
	for _, model := range models {
		if model.HasStringPrimaryID {
			return true
		}
	}
	return false
}

// getFieldType check's if user has defined a
func getFieldType(binder *config.Binder, schema *ast.Schema, cfg *config.Config, field *ast.FieldDefinition) (types.Type, error) {
	var typ types.Type
	var err error

	fieldDef := schema.Types[field.Type.Name()]
	if cfg.Models.UserDefined(field.Type.Name()) {
		typ, err = binder.FindTypeFromName(cfg.Models[field.Type.Name()].Model[0])
		if err != nil {
			return typ, err
		}
	} else {
		switch fieldDef.Kind {
		case ast.Scalar:
			// no user defined model, referencing a default scalar
			typ = types.NewNamed(
				types.NewTypeName(0, cfg.Model.Pkg(), "string", nil),
				nil,
				nil,
			)

		case ast.Interface, ast.Union:
			// no user defined model, referencing a generated interface type
			typ = types.NewNamed(
				types.NewTypeName(0, cfg.Model.Pkg(), templates.ToGo(field.Type.Name()), nil),
				types.NewInterfaceType([]*types.Func{}, []types.Type{}),
				nil,
			)

		case ast.Enum:
			// no user defined model, must reference a generated enum
			typ = types.NewNamed(
				types.NewTypeName(0, cfg.Model.Pkg(), templates.ToGo(field.Type.Name()), nil),
				nil,
				nil,
			)

		case ast.Object, ast.InputObject:
			// no user defined model, must reference a generated struct
			typ = types.NewNamed(
				types.NewTypeName(0, cfg.Model.Pkg(), templates.ToGo(field.Type.Name()), nil),
				types.NewStruct(nil, nil),
				nil,
			)

		default:
			panic(fmt.Errorf("unknown ast type %s", fieldDef.Kind))
		}
	}

	return typ, err
}

func getPlularBoilerRelationShipName(modelName string) string {
	// sqlboiler adds Slice when multiple, we don't want that
	// since our converts are named plular of model and not Slice
	// e.g. UsersToGraphQL and not UserSliceToGraphQL
	modelName = strings.TrimSuffix(modelName, "Slice")
	return pluralizer.Plural(modelName)
}

func enhanceModelsWithFields(enums []*Enum, schema *ast.Schema, cfg *config.Config, models []*Model) {

	binder := cfg.NewBinder()

	// Generate the basic of the fields
	for _, m := range models {

		// Let's convert the pure ast fields to something usable for our template
		for _, field := range m.PureFields {
			fieldDef := schema.Types[field.Type.Name()]

			// This calls some qglgen boilerType which gets the gqlgen type
			typ, err := getFieldType(binder, schema, cfg, field)
			if err != nil {
				fmt.Println("Could not get field type from graphql schema: ", err)
			}

			name := field.Name
			if nameOveride := cfg.Models[m.Name].Fields[field.Name].FieldName; nameOveride != "" {
				// TODO: map overrides to sqlboiler the other way around?
				name = nameOveride
			}

			// just some (old) Relay clutter which is not needed anymore + we won't do anything with it
			// in our database converts.
			if name == "clientMutationId" {
				continue
			}

			// override type struct with qqlgen code
			typ = binder.CopyModifiersFromAst(field.Type, typ)
			if isStruct(typ) && (fieldDef.Kind == ast.Object || fieldDef.Kind == ast.InputObject) {
				typ = types.NewPointer(typ)
			}

			// get golang friendly fieldName because we want to check if boiler name is available
			golangName := getGoFieldName(name)

			// generate some booleans because these checks will be used a lot
			isRelation := fieldDef.Kind == ast.Object || fieldDef.Kind == ast.InputObject

			shortType := getShortType(typ.String())

			isPrimaryID := golangName == "ID"

			// get sqlboiler information of the field
			boilerField := findBoilerFieldOrForeignKey(m.BoilerModel.Fields, golangName, isRelation)
			isString := strings.Contains(strings.ToLower(boilerField.Type), "string")
			isNumberID := strings.Contains(golangName, "ID") && !isString
			isPrimaryNumberID := isPrimaryID && !isString

			isPrimaryStringID := isPrimaryID && isString
			// enable simpler code in resolvers

			if isPrimaryStringID {
				m.HasStringPrimaryID = isPrimaryStringID
			}
			if isPrimaryNumberID || isPrimaryStringID {
				m.PrimaryKeyType = boilerField.Type
			}

			// log some warnings when fields could not be converted
			if boilerField.Type == "" {
				// TODO: add filter + where here
				if m.IsPayload {
					// ignore
				} else if pluralizer.IsPlural(name) {
					// ignore
				} else if (m.IsFilter || m.IsWhere) && (name == "and" || name == "or" || name == "search" ||
					name == "where") {
					// ignore
				} else {
					fmt.Println("[WARN] boiler type not available for ", name)
				}
			}

			if boilerField.Name == "" {
				if m.IsPayload || m.IsFilter || m.IsWhere {
				} else {
					fmt.Println("[WARN] boiler name not available for ", m.Name+"."+golangName)
					continue
				}

			}
			field := &Field{
				Name:               name,
				Type:               shortType,
				TypeWithoutPointer: strings.Replace(strings.TrimPrefix(shortType, "*"), ".", "Dot", -1),
				BoilerField:        boilerField,
				IsNumberID:         isNumberID,
				IsPrimaryID:        isPrimaryID,
				IsPrimaryNumberID:  isPrimaryNumberID,
				IsRelation:         isRelation,
				IsOr:               name == "or",
				IsAnd:              name == "and",
				IsPlural:           pluralizer.IsPlural(name),
				PluralName:         pluralizer.Plural(name),
				OriginalType:       typ,
				Description:        field.Description,
				Tag:                `json:"` + field.Name + `"`,
			}
			field.ConvertConfig = getConvertConfig(enums, m, field)
			m.Fields = append(m.Fields, field)

		}
	}

	for _, m := range models {
		m.HasOrganizationID = findField(m.Fields, "organizationId") != nil
		m.HasUserOrganizationID = findField(m.Fields, "userOrganizationId") != nil
		m.HasUserID = findField(m.Fields, "userId") != nil
		for _, f := range m.Fields {
			f.Relationship = findModel(models, f.BoilerField.Relationship.Name)
		}
	}
}

var ignoreTypePrefixes = []string{"graphql_models", "models", "gqlutils"}

func getShortType(longType string) string {

	// longType e.g = gitlab.com/decicify/app/backend/graphql_models.FlowWhere
	splittedBySlash := strings.Split(longType, "/")
	// gitlab.com, decicify, app, backend, graphql_models.FlowWhere

	lastPart := splittedBySlash[len(splittedBySlash)-1]
	isPointer := strings.HasPrefix(longType, "*")
	isStructInPackage := strings.Count(lastPart, ".") > 0

	if isStructInPackage {
		// if packages are deeper they don't have pointers but *time.Time will since it's not deep
		returnType := strings.TrimPrefix(lastPart, "*")
		for _, ignoreType := range ignoreTypePrefixes {
			fullIgnoreType := ignoreType + "."
			returnType = strings.TrimPrefix(returnType, fullIgnoreType)
		}

		if isPointer {
			return "*" + returnType
		}
		return returnType
	}

	return longType
}

func findModel(models []*Model, search string) *Model {
	for _, m := range models {
		if m.Name == search {
			return m
		}
	}
	return nil
}

func findField(fields []*Field, search string) *Field {
	for _, f := range fields {
		if f.Name == search {
			return f
		}
	}
	return nil
}
func findRelationModelForForeignKeyAndInput(currentModelName string, foreignKey string, models []*Model) *Field {
	return findRelationModelForForeignKey(getBaseModelFromName(currentModelName), foreignKey, models)
}

func findRelationModelForForeignKey(currentModelName string, foreignKey string, models []*Model) *Field {

	model := findModel(models, currentModelName)
	if model != nil {
		// Use case
		// we want a foreignKey of ParentID but the foreign key resolves to Calamity
		// We could know this based on the boilerType information
		// withou this function the generated convert is like this

		// r.Parent = ParentToGraphQL(m.R.Parent, m)
		// but it needs to be
		// r.Parent = CalamityToGraphQL(m.R.Parent, m)
		foreignKey = strings.TrimSuffix(foreignKey, "Id")

		field := findField(model.Fields, foreignKey)
		if field != nil {
			// fmt.Println("Found graph type", field.Name, "for foreign key", foreignKey)
			return field
		}
	}

	return nil
}

func findBoilerFieldOrForeignKey(fields []*BoilerField, golangGraphQLName string, isRelation bool) BoilerField {
	// get database friendly struct for this model
	for _, field := range fields {
		if isRelation {
			// If it a relation check to see if a foreign key is available
			if field.Name == golangGraphQLName+"ID" {
				return *field
			}
		}
		if field.Name == golangGraphQLName {
			return *field
		}
	}

	// // fallback on foreignKey

	// }

	// fmt.Println("???", golangGraphQLName)

	return BoilerField{}
}

func getGoFieldName(name string) string {
	goFieldName := strcase.ToCamel(name)
	// in golang Id = ID
	goFieldName = strings.Replace(goFieldName, "Id", "ID", -1)
	// in golang Url = URL
	goFieldName = strings.Replace(goFieldName, "Url", "URL", -1)
	return goFieldName
}

func getExtrasFromSchema(schema *ast.Schema) (interfaces []*Interface, enums []*Enum, scalars []string) {
	for _, schemaType := range schema.Types {
		switch schemaType.Kind {
		case ast.Interface, ast.Union:
			interfaces = append(interfaces, &Interface{
				Description: schemaType.Description,
				Name:        schemaType.Name,
			})
		case ast.Enum:
			it := &Enum{
				Name: schemaType.Name,

				Description: schemaType.Description,
			}
			for _, v := range schemaType.EnumValues {
				it.Values = append(it.Values, &EnumValue{
					Name:        v.Name,
					NameLower:   strcase.ToLowerCamel(strings.ToLower(v.Name)),
					Description: v.Description,
				})
			}
			if strings.HasPrefix(it.Name, "_") {
				continue
			}
			enums = append(enums, it)
		case ast.Scalar:
			scalars = append(scalars, schemaType.Name)
		}
	}
	return
}

func getModelsFromSchema(schema *ast.Schema, boilerModels []*BoilerModel) (models []*Model) {
	for _, schemaType := range schema.Types {

		// skip boiler plate from ggqlgen, we only want the models
		if strings.HasPrefix(schemaType.Name, "_") {
			continue
		}

		// if cfg.Models.UserDefined(schemaType.Name) {
		// 	fmt.Println("continue")
		// 	continue
		// }

		switch schemaType.Kind {

		case ast.Object, ast.InputObject:
			{
				if schemaType == schema.Query ||
					schemaType == schema.Mutation ||
					schemaType == schema.Subscription {
					continue
				}
				modelName := schemaType.Name

				// fmt.Println("GRAPHQL MODEL ::::", m.Name)
				if strings.HasPrefix(modelName, "_") {
					continue
				}

				// We will try to find a corresponding boiler struct
				boilerModel := FindBoilerModel(boilerModels, getBaseModelFromName(modelName))

				isInput := strings.HasSuffix(modelName, "Input") && modelName != "Input"
				isCreateInput := strings.HasSuffix(modelName, "CreateInput") && modelName != "CreateInput"
				isUpdateInput := strings.HasSuffix(modelName, "UpdateInput") && modelName != "UpdateInput"
				isFilter := strings.HasSuffix(modelName, "Filter") && modelName != "Filter"
				isWhere := strings.HasSuffix(modelName, "Where") && modelName != "Where"
				isPayload := strings.HasSuffix(modelName, "Payload") && modelName != "Payload"

				// if no boiler model is found
				if boilerModel.Name == "" {
					if isInput || isWhere || isFilter || isPayload {
						// silent continue
						continue
					}

					fmt.Println(fmt.Sprintf("[WARN] Skip %v because no database model found", modelName))
					continue
				}

				isNormalInput := isInput && !isCreateInput && !isUpdateInput

				m := &Model{
					Name:          modelName,
					Description:   schemaType.Description,
					PluralName:    pluralizer.Plural(modelName),
					BoilerModel:   boilerModel,
					IsInput:       isInput,
					IsFilter:      isFilter,
					IsWhere:       isWhere,
					IsUpdateInput: isUpdateInput,
					IsCreateInput: isCreateInput,
					IsNormalInput: isNormalInput,
					IsPayload:     isPayload,
					IsNormal:      !isInput && !isWhere && !isFilter && !isPayload,
					IsPreloadable: !isInput && !isWhere && !isFilter && !isPayload,
				}

				for _, implementor := range schema.GetImplements(schemaType) {
					m.Implements = append(m.Implements, implementor.Name)
				}

				m.PureFields = append(m.PureFields, schemaType.Fields...)
				models = append(models, m)
			}
		}
	}
	return
}

func getPreloadMapForModel(model *Model) map[string]ColumnSetting {
	preloadMap := map[string]ColumnSetting{}
	for _, field := range model.Fields {
		// only relations are preloadable
		if !field.IsRelation {
			continue
		}
		// var key string
		// if field.IsPlural {
		key := field.Name
		// } else {
		// 	key = field.PluralName
		// }
		name := fmt.Sprintf("models.%vRels.%v", model.Name, foreignKeyToRel(field.BoilerField.Name))
		setting := ColumnSetting{
			Name:                  name,
			IDAvailable:           !field.IsPlural,
			RelationshipModelName: field.BoilerField.Relationship.Name,
		}

		preloadMap[key] = setting
	}
	return preloadMap
}

const maximumLevelOfPreloads = 4

func enhanceModelsWithPreloadArray(models []*Model) {

	// first adding basic first level relations
	for _, model := range models {
		if !model.IsPreloadable {
			continue
		}

		modelPreloadMap := getPreloadMapForModel(model)

		sortedPreloadKeys := make([]string, 0, len(modelPreloadMap))
		for k := range modelPreloadMap {
			sortedPreloadKeys = append(sortedPreloadKeys, k)
		}
		sort.Strings(sortedPreloadKeys)

		model.PreloadArray = make([]Preload, len(sortedPreloadKeys))
		for i, k := range sortedPreloadKeys {
			columnSetting := modelPreloadMap[k]
			model.PreloadArray[i] = Preload{
				Key:           k,
				ColumnSetting: columnSetting,
			}
		}
	}
}

func enhancePreloadMapWithNestedRelations(
	fullMap map[string]map[string]ColumnSetting,
	preloadMapPerModel map[string]map[string]ColumnSetting,
	modelName string,
) {

	for key, value := range preloadMapPerModel[modelName] {

		// check if relation exist
		if value.RelationshipModelName != "" {
			nestedPreloads, ok := fullMap[value.RelationshipModelName]
			if ok {
				for nestedKey, nestedValue := range nestedPreloads {

					newKey := key + `.` + nestedKey

					if strings.Count(newKey, ".") > maximumLevelOfPreloads {
						continue
					}
					fullMap[modelName][newKey] = ColumnSetting{
						Name:                  value.Name + `+ "." +` + nestedValue.Name,
						RelationshipModelName: nestedValue.RelationshipModelName,
					}
				}
			}
		}
	}
}

// The relationship is defined in the normal model but not in the input, where etc structs
// So just find the normal model and get the relationship type :)
func getBaseModelFromName(v string) string {
	v = safeTrim(v, "CreateInput")
	v = safeTrim(v, "UpdateInput")
	v = safeTrim(v, "Input")
	v = safeTrim(v, "Payload")
	v = safeTrim(v, "Where")
	v = safeTrim(v, "Filter")
	return v
}

func safeTrim(v string, trimSuffix string) string {
	// let user still choose Payload as model names
	// not recommended but could be done theoretically :-)
	if v != trimSuffix {
		v = strings.TrimSuffix(v, trimSuffix)
	}
	return v
}

func foreignKeyToRel(v string) string {
	return strings.TrimSuffix(strcase.ToCamel(v), "ID")
}

func isStruct(t types.Type) bool {
	_, is := t.Underlying().(*types.Struct)
	return is
}

type ConvertConfig struct {
	IsCustom         bool
	ToBoiler         string
	ToGraphQL        string
	GraphTypeAsText  string
	BoilerTypeAsText string
}

func findEnum(enums []*Enum, graphType string) *Enum {
	for _, enum := range enums {
		if enum.Name == graphType {
			return enum
		}
	}
	return nil
}

func getConvertConfig(enums []*Enum, model *Model, field *Field) (cc ConvertConfig) {
	graphType := field.Type
	boilType := field.BoilerField.Type

	enum := findEnum(enums, field.TypeWithoutPointer)
	if enum != nil {
		cc.IsCustom = true
		cc.ToBoiler = strings.TrimPrefix(
			getToBoiler(
				getBoilerTypeAsText(boilType),
				getGraphTypeAsText(graphType),
			), "gqlutils.")

		cc.ToGraphQL = strings.TrimPrefix(
			getToGraphQL(
				getBoilerTypeAsText(boilType),
				getGraphTypeAsText(graphType),
			), "gqlutils.")

	} else if graphType != boilType {
		cc.IsCustom = true

		if field.IsPrimaryNumberID || field.IsNumberID {

			cc.ToGraphQL = "VALUE"
			cc.ToBoiler = "VALUE"

			// first unpointer json type if is pointer
			if strings.HasPrefix(graphType, "*") {
				cc.ToBoiler = "gqlutils.PointerStringToString(VALUE)"
			}

			goToUint := getBoilerTypeAsText(boilType) + "ToUint"
			if goToUint == "IntToUint" {
				cc.ToGraphQL = "uint(VALUE)"
			} else if goToUint != "UintToUint" {
				cc.ToGraphQL = "gqlutils." + goToUint + "(VALUE)"
			}

			if field.IsPrimaryNumberID {
				cc.ToGraphQL = model.Name + "IDToGraphQL(" + cc.ToGraphQL + ")"
			} else if field.IsNumberID {
				cc.ToGraphQL = field.BoilerField.Relationship.Name + "IDToGraphQL(" + cc.ToGraphQL + ")"
			}

			isInt := strings.HasPrefix(strings.ToLower(boilType), "int") && !strings.HasPrefix(strings.ToLower(boilType), "uint")

			if strings.HasPrefix(boilType, "null") {
				cc.ToBoiler = fmt.Sprintf("gqlutils.IDToNullBoiler(%v)", cc.ToBoiler)
				if isInt {
					cc.ToBoiler = fmt.Sprintf("gqlutils.NullUintToNullInt(%v)", cc.ToBoiler)
				}

			} else {
				cc.ToBoiler = fmt.Sprintf("gqlutils.IDToBoiler(%v)", cc.ToBoiler)
				if isInt {
					cc.ToBoiler = fmt.Sprintf("int(%v)", cc.ToBoiler)
				}
			}

			cc.ToGraphQL = strings.Replace(cc.ToGraphQL, "VALUE", "m."+getGoFieldName(field.BoilerField.Name), -1)
			cc.ToBoiler = strings.Replace(cc.ToBoiler, "VALUE", "m."+getGoFieldName(field.Name), -1)

		} else {
			// Make these go-friendly for the helper/convert.go package
			cc.ToBoiler = getToBoiler(getBoilerTypeAsText(boilType), getGraphTypeAsText(graphType))
			cc.ToGraphQL = getToGraphQL(getBoilerTypeAsText(boilType), getGraphTypeAsText(graphType))
		}

	}
	// fmt.Println("boilType for", field.Name, ":", boilType)

	cc.GraphTypeAsText = getGraphTypeAsText(graphType)
	cc.BoilerTypeAsText = getBoilerTypeAsText(boilType)

	return
}

func getToBoiler(boilType, graphType string) string {
	return "gqlutils." + getGraphTypeAsText(graphType) + "To" + getBoilerTypeAsText(boilType)
}

func getToGraphQL(boilType, graphType string) string {
	return "gqlutils." + getBoilerTypeAsText(boilType) + "To" + getGraphTypeAsText(graphType)
}

func getBoilerTypeAsText(boilType string) string {

	// backward compatible missed Dot
	if strings.HasPrefix(boilType, "types.") {
		boilType = strings.TrimPrefix(boilType, "types.")
		boilType = strcase.ToCamel(boilType)
		boilType = "Types" + boilType
	}

	// if strings.HasPrefix(boilType, "null.") {
	// 	boilType = strings.TrimPrefix(boilType, "null.")
	// 	boilType = strcase.ToCamel(boilType)
	// 	boilType = "NullDot" + boilType
	// }
	boilType = strings.Replace(boilType, ".", "Dot", -1)

	return strcase.ToCamel(boilType)
}

func getGraphTypeAsText(graphType string) string {
	if strings.HasPrefix(graphType, "*") {
		graphType = strings.TrimPrefix(graphType, "*")
		graphType = strcase.ToCamel(graphType)
		graphType = "Pointer" + graphType
	}
	return strcase.ToCamel(graphType)
}
