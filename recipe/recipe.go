package recipe

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type Output struct {
	Type  DataType
	Value string
}

func (o *Output) GetValue(ctx LineContext) (string, error) {
	if o.Type == Variable {
		value, ok := ctx.Variables[o.Value]
		if !ok {
			return "", errors.New("unrecognized variable")
		}
		return value, nil
	}
	if o.Type == Column {
		column, _ := strconv.Atoi(o.Value)
		value, ok := ctx.Columns[column]
		if !ok {
			return "", errors.New("unrecognized/unfilled column number")
		}
		return value, nil
	}

	return "", errors.New("unknown column type")
}

type Argument struct {
	Type  DataType
	Value string
}

func (a *Argument) GetValue(context LineContext, placeholder string) (string, error) {
	var value string
	switch a.Type {
	case Column:
		colNum, _ := strconv.Atoi(a.Value)
		colValue, ok := context.Columns[colNum]
		if !ok {
			return "", fmt.Errorf("column %d referenced, but it does not exist in the input", colNum)
		}
		value = colValue
	case Variable:
		varValue, ok := context.Variables[a.Value]
		if !ok {
			return "", fmt.Errorf("variable '%s' referenced, but it is not defined", a.Value)
		}
		value = varValue
	case Literal:
		return a.Value, nil
	case Placeholder:
		return placeholder, nil
	default:
		return "", fmt.Errorf("argument GetValue not implemented for type %s", a.Type.String())
	}

	return value, nil
}

type Operation struct {
	Name      string
	Arguments []Argument
}

type Recipe struct {
	Output  Output
	Pipe    []Operation
	Comment string
}

type Transformation struct {
	Variables     map[string]Recipe
	Columns       map[int]Recipe
	Headers       map[int]Recipe
	VariableOrder []string
}

type TransformationResult struct {
	HeaderLines int
	Lines       int
}

func (t *Transformation) Dump(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Headers: \n=====")
	for _, h := range t.Headers {
		_, _ = fmt.Fprintf(w, "Header: %s\n", h.Output.Value)
		_, _ = fmt.Fprintf(w, "pipe: ")
		for _, p := range h.Pipe {
			_, _ = fmt.Fprintf(w, p.Name+"(")
			for _, a := range p.Arguments {
				_, _ = fmt.Fprintf(w, "%s: %s, ", a.Type.String(), a.Value)
			}
			_, _ = fmt.Fprintf(w, ") -> ")
		}
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "Comment: # %s\n---\n", h.Comment)
	}

	_, _ = fmt.Fprintln(w, "Variables: \n======")
	for _, v := range t.Variables {
		_, _ = fmt.Fprintf(w, "Var: %s\n", v.Output.Value)
		_, _ = fmt.Fprint(w, "pipe: ")
		for _, p := range v.Pipe {
			_, _ = fmt.Fprint(w, p.Name+"(")
			for _, a := range p.Arguments {
				_, _ = fmt.Fprintf(w, "%s: %s, ", a.Type.String(), a.Value)
			}
			_, _ = fmt.Fprintf(w, ") -> ")
		}
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "Comment: %s\n---\n", v.Comment)
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Columns: \n======")
	for _, c := range t.Columns {
		_, _ = fmt.Fprintf(w, "Column: %s\n", c.Output.Value)
		_, _ = fmt.Fprint(w, "pipe: ")
		for _, p := range c.Pipe {
			_, _ = fmt.Fprintf(w, p.Name+"(")
			for _, a := range p.Arguments {
				_, _ = fmt.Fprintf(w, "%s: %s, ", a.Type.String(), a.Value)
			}
			_, _ = fmt.Fprint(w, ") -> ")
		}
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "Comment: %s\n---\n", c.Comment)
	}
}

func (t *Transformation) AddOutputToVariable(variable string) error {
	_, ok := t.Variables[variable]
	if ok {
		return fmt.Errorf("variable %s already defined", variable)
	}
	t.Variables[variable] = Recipe{Output: getOutputForVariable(variable)}
	return nil
}

func (t *Transformation) AddOutputToColumn(column string) error {
	output := getOutputForColumn(column)
	columnNum, _ := strconv.Atoi(column)
	_, ok := t.Columns[columnNum]
	if ok {
		return fmt.Errorf("column %d already defined", columnNum)
	}
	t.Columns[columnNum] = Recipe{Output: output}
	return nil
}

func (t *Transformation) AddOutputToHeader(header string) error {
	output := getOutputForHeader(header)
	headerNum, _ := strconv.Atoi(header)
	_, ok := t.Headers[headerNum]
	if ok {
		return fmt.Errorf("header %d already defined", headerNum)
	}
	t.Headers[headerNum] = Recipe{Output: output}
	return nil
}

func (t *Transformation) Execute(reader *csv.Reader, writer *csv.Writer, processHeader bool, lineLimit int) (*TransformationResult, error) {
	defer writer.Flush()

	numColumns := len(t.Columns)

	if err := t.ValidateRecipe(); err != nil {
		return nil, err
	}
	var linesRead int

	for {
		if lineLimit > 0 && linesRead >= lineLimit {
			break
		}
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		linesRead++

		var context = LineContext{
			Variables: map[string]string{},
			Columns:   map[int]string{},
			LineNo:    linesRead,
		}
		// Load context with all the columns
		for i, v := range row {
			context.Columns[i+1] = v
		}

		// process variables
		for _, v := range t.VariableOrder {
			variableName := t.Variables[v].Output.Value
			variableRecipe := t.Variables[v]
			placeholder, err := t.processRecipe("variable", variableRecipe, context)
			if err != nil {
				return nil, err
			}
			context.Variables[variableName] = placeholder
		}

		if processHeader && linesRead == 1 {
			// Load existing headers up to size of output
			var output = make(map[int]string)
			for i := 1; i <= numColumns; i++ {
				var value string
				if i <= len(row) {
					value = row[i-1]
				} else {
					value = fmt.Sprintf("column %d", i)
				}
				output[i] = value
			}

			for h := range t.Headers {
				headerRecipe := t.Headers[h]
				placeholder, err := t.processRecipe("header", headerRecipe, context)
				if err != nil {
					return nil, err
				}
				output[h] = placeholder
			}

			err := t.outputCsvRow(numColumns, output, writer)
			if err != nil {
				return nil, err
			}
		}

		if !processHeader || linesRead > 1 {
			var output = make(map[int]string)

			for c := range t.Columns {
				columnRecipe := t.Columns[c]
				placeholder, err := t.processRecipe("column", columnRecipe, context)
				if err != nil {
					return nil, err
				}
				output[c] = placeholder
			}

			err = t.outputCsvRow(numColumns, output, writer)
			if err != nil {
				return nil, err
			}
		}

		if linesRead%100 == 0 {
			writer.Flush()
		}
	}

	var headerLines int
	if processHeader {
		headerLines = 1
	}

	result := TransformationResult{
		Lines:       linesRead - headerLines,
		HeaderLines: headerLines,
	}

	return &result, nil
}

func (t *Transformation) outputCsvRow(numColumns int, output map[int]string, writer *csv.Writer) error {
	var outputRow []string
	for i := 1; i <= numColumns; i++ {
		outputRow = append(outputRow, output[i])
	}
	err := writer.Write(outputRow)
	if err != nil {
		return err
	}
	return nil
}

func (t *Transformation) processRecipe(recipeType string, variable Recipe, context LineContext) (string, error) {
	var placeholder string
	var value string
	mode := Replace

	errorPrefix := fmt.Sprintf("line %d / %s %s:", context.LineNo, recipeType, variable.Output.Value)

	for _, o := range variable.Pipe {
		opName := strings.ToLower(o.Name)
		switch opName {
		case "value":
			firstArg := o.Arguments[0]
			argValue, err := firstArg.GetValue(context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %v", errorPrefix, err)
			}
			value = argValue
		case "join":
			firstArg := o.Arguments[0]
			mode = Join
			argValue, err := firstArg.GetValue(context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %v", errorPrefix, err)
			}
			value = argValue
			// If the argument is placeholder then there's something coming after
			if firstArg.Type == Placeholder {
				continue
			}
		case "uppercase":
			firstArg, err := o.Arguments[0].GetValue(context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			value = Uppercase(firstArg)
		case "lowercase":
			firstArg, err := o.Arguments[0].GetValue(context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			value = Lowercase(firstArg)
		case "add":
			args, err := processArgs(2, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			sum, err := Add(args[0], args[1])
			if err != nil {
				return "", fmt.Errorf("%s %s(): %v", errorPrefix, opName, err)
			}
			value = sum
		case "subtract":
			args, err := processArgs(2, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			difference, err := Subtract(args[0], args[1])
			if err != nil {
				return "", fmt.Errorf("%s %s(): %v", errorPrefix, opName, err)
			}
			value = difference
		case "multiply":
			args, err := processArgs(2, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			product, err := Multiply(args[0], args[1])
			if err != nil {
				return "", fmt.Errorf("%s %s(): %v", errorPrefix, opName, err)
			}
			value = product
		case "divide":
			args, err := processArgs(2, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			product, err := Divide(args[0], args[1])
			if err != nil {
				return "", fmt.Errorf("%s %s(): %v", errorPrefix, opName, err)
			}
			value = product
		case "change":
			args, err := processArgs(3, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			updated, _ := Change(args[0], args[1], args[2]) // no errors from this
			value = updated
		case "changei":
			args, err := processArgs(3, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			updated, _ := ChangeI(args[0], args[1], args[2]) // no errors from this
			value = updated
		case "ifempty", "isempty":
			args, err := processArgs(3, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			result, _ := IfEmpty(args[0], args[1], args[2]) // no errors
			value = result
		case "numberformat":
			args, err := processArgs(3, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			result, err := NumberFormat(args[0], args[1])
			if err != nil {
				return "", fmt.Errorf("%s %s(): %v", errorPrefix, opName, err)
			}
			value = result
		case "lineno":
			value = strconv.Itoa(context.LineNo)
		case "removedigits":
			args, err := processArgs(1, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			result, _ := RemoveDigits(args[0]) // no errors from this
			value = result
		case "onlydigits":
			args, err := processArgs(1, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			result, _ := OnlyDigits(args[0]) // no errors from this
			value = result
		case "mod":
			args, err := processArgs(2, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			result, err := Modulus(args[0], args[1])
			if err != nil {
				return "", fmt.Errorf("%s %s(): %v", errorPrefix, opName, err)
			}
			value = result
		case "trim":
			args, err := processArgs(1, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			result, _ := Trim(args[0])
			value = result
		case "firstchars":
			args, err := processArgs(2, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			result, err := FirstChars(args[0], args[1])
			if err != nil {
				return "", fmt.Errorf("%s %s(): %v", errorPrefix, opName, err)
			}
			value = result
		case "lastchars":
			args, err := processArgs(2, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			result, err := LastChars(args[0], args[1])
			if err != nil {
				return "", fmt.Errorf("%s %s(): %v", errorPrefix, opName, err)
			}
			value = result
		case "repeat":
			args, err := processArgs(2, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			result, err := Repeat(args[0], args[1])
			if err != nil {
				return "", fmt.Errorf("%s %s(): %v", errorPrefix, opName, err)
			}
			value = result
		case "replace":
			args, err := processArgs(3, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			result, _ := ReplaceString(args[0], args[1], args[2]) // no errors from this
			value = result
		case "today":
			value, _ = Today(Now)
		case "now":
			value, _ = NowTime(Now)
		case "formatdate":
			args, err := processArgs(2, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			result, err := FormatDate(args[0], args[1])
			if err != nil {
				return "", fmt.Errorf("%s %s(): %v", errorPrefix, opName, err)
			}
			value = result
		case "formatdatef":
			args, err := processArgs(2, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			result, err := FormatDateF(args[0], args[1])
			if err != nil {
				return "", fmt.Errorf("%s %s(): %v", errorPrefix, opName, err)
			}
			value = result
		case "readdate":
			args, err := processArgs(2, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			result, err := ReadDate(args[0], args[1])
			if err != nil {
				return "", fmt.Errorf("%s %s(): %v", errorPrefix, opName, err)
			}
			value = result
		case "readdatef":
			args, err := processArgs(2, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			result, err := ReadDateF(args[0], args[1])
			if err != nil {
				return "", fmt.Errorf("%s %s(): %v", errorPrefix, opName, err)
			}
			value = result
		case "smartdate":
			args, err := processArgs(1, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			result, err := SmartDate(args[0])
			if err != nil {
				return "", fmt.Errorf("%s %s(): %v", errorPrefix, opName, err)
			}
			value = result
		case "ispast":
			args, err := processArgs(3, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			result, err := IsPast(args[0], args[1], args[2])
			if err != nil {
				return "", fmt.Errorf("%s %s(): %v", errorPrefix, opName, err)
			}
			value = result
		case "isfuture":
			args, err := processArgs(3, o.Arguments, context, placeholder)
			if err != nil {
				return "", fmt.Errorf("%s %s(): error evaluating arg: %v", errorPrefix, opName, err)
			}
			result, err := IsFuture(args[0], args[1], args[2])
			if err != nil {
				return "", fmt.Errorf("%s %s(): %v", errorPrefix, opName, err)
			}
			value = result
		// TODO make function calling more smart, using the allFuncs thing
		default:
			return "", fmt.Errorf("%s error: processing variable, unimplemented operation %s", errorPrefix, o.Name)
		}

		switch mode {
		case Replace:
			placeholder = value
		case Join:
			placeholder += value
			mode = Replace
		default:
			return "", fmt.Errorf("invalid join mode %d", mode)
		}
	}
	return placeholder, nil
}

func processArgs(numArgs int, arguments []Argument, context LineContext, placeholder string) ([]string, error) {
	for len(arguments) < numArgs {
		arguments = append(arguments, getPlaceholderArg())
	}

	var processedArgs []string

	for i := 0; i < numArgs; i++ {
		value, err := arguments[i].GetValue(context, placeholder)
		if err != nil {
			return []string{}, err
		}
		processedArgs = append(processedArgs, value)
	}

	return processedArgs, nil
}

func getPlaceholderArg() Argument {
	return Argument{
		Type:  Placeholder,
		Value: "?",
	}
}

func (t *Transformation) AddOperationToVariable(variable string, operation Operation) {
	recipe, ok := t.Variables[variable]
	if !ok {
		t.AddOutputToVariable(variable)
		recipe = t.Variables[variable]
	}
	pipe := recipe.Pipe
	if pipe == nil {
		pipe = []Operation{}
	}
	pipe = append(pipe, operation)
	recipe.Pipe = pipe
	t.Variables[variable] = recipe
}

func (t *Transformation) AddOperationToColumn(column string, operation Operation) {
	columnNumber, _ := strconv.Atoi(column)
	recipe, ok := t.Columns[columnNumber]
	if !ok {
		t.AddOutputToColumn(column)
		recipe = t.Columns[columnNumber]
	}
	pipe := recipe.Pipe
	if pipe == nil {
		pipe = []Operation{}
	}
	pipe = append(pipe, operation)
	recipe.Pipe = pipe
	t.Columns[columnNumber] = recipe
}

func (t *Transformation) AddOperationToHeader(header string, operation Operation) {
	headerNumber, _ := strconv.Atoi(header)
	recipe, ok := t.Headers[headerNumber]
	if !ok {
		t.AddOutputToHeader(header)
		recipe = t.Headers[headerNumber]
	}
	pipe := recipe.Pipe
	if pipe == nil {
		pipe = []Operation{}
	}
	pipe = append(pipe, operation)
	recipe.Pipe = pipe
	t.Headers[headerNumber] = recipe
}

func (t *Transformation) AddOperationByType(targetType DataType, target string, operation Operation) {
	switch targetType {
	case Variable:
		t.AddOperationToVariable(target, operation)
	case Column:
		t.AddOperationToColumn(target, operation)
	case Header:
		t.AddOperationToHeader(target, operation)
	}
}

func (t *Transformation) ValidateRecipe() error {
	numColumns := len(t.Columns)

	// recipe with no columns is pointless/invalid
	if numColumns == 0 {
		return errors.New("no column recipes provided")
	}

	// validate all columns are specified
	for c := 1; c <= numColumns; c++ {
		if _, ok := t.Columns[c]; !ok {
			return fmt.Errorf("missing column definition for column #%d", c)
		}
	}

	// ensure there are not header recipes for a column we don't have
	for h := range t.Headers {
		if _, ok := t.Columns[h]; !ok {
			return fmt.Errorf("found header for column %d, but no recipe for column %d", h, h)
		}
	}

	return nil
}

type LineContext struct {
	Variables map[string]string
	Columns   map[int]string
	LineNo    int
}

func NewTransformation() *Transformation {
	return &Transformation{
		Variables: make(map[string]Recipe),
		Columns:   make(map[int]Recipe),
		Headers:   make(map[int]Recipe),
	}
}
