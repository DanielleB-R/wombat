package decode

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/DanielleB-R/wombat/wasm"
)

var wasmHeader = []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}

// ReadSections reads the sections of a WASM module out of a reader without decoding the sections
func ReadSections(reader *bytes.Reader) (*wasm.RawModule, error) {
	var module wasm.RawModule

	for _, value := range wasmHeader {
		err := expectByte(reader, value)
		if err != nil {
			return nil, err
		}
	}

	lastID := byte(0)

	for reader.Len() > 0 {
		id, err := reader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("error reading section id: %w", err)
		}

		if id != 0 && id <= lastID {
			return nil, errors.New("section out of order")
		}
		if id > 11 {
			return nil, errors.New("unknown section type")
		}
		if id != 0 {
			lastID = id
		}

		length, err := ReadLEB128U32(reader)
		if err != nil {
			return nil, fmt.Errorf("error reading section length: %w", err)
		}

		if length > uint32(reader.Len()) {
			return nil, errors.New("section length too long")
		}

		contents := make([]byte, length)
		n, err := reader.Read(contents)
		if n != int(length) {
			return nil, errors.New("contents unavailable")
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("error reading section contents, %w", err)
		}

		if id == 0 {
			_, err := readName(bytes.NewReader(contents))
			if err != nil {
				return nil, err
			}
			continue
		}

		module.Sections = append(module.Sections, wasm.RawSection{ID: id, Length: length, Contents: contents})
	}

	return &module, nil
}

func DecodeSections(raw *wasm.RawModule) (*wasm.Module, error) {
	var module wasm.Module

	for _, rawSection := range raw.Sections {
		switch rawSection.ID {
		case 1:
			typeSection, err := decodeTypeSection(rawSection.Contents)
			if err != nil {
				return nil, fmt.Errorf("error decoding type section: %w", err)
			}
			module.Type = typeSection
		case 2:
			importSection, err := decodeImportSection(rawSection.Contents)
			if err != nil {
				return nil, fmt.Errorf("error decoding import section: %w", err)
			}
			module.Import = importSection
		case 3:
			functionSection, err := decodeFunctionSection(rawSection.Contents)
			if err != nil {
				return nil, fmt.Errorf("error decoding function section: %w", err)
			}
			module.Function = functionSection

		case 4:
			tableSection, err := decodeTableSection(rawSection.Contents)
			if err != nil {
				return nil, fmt.Errorf("error decoding table section: %w", err)
			}
			module.Table = tableSection
		case 5:
			memorySection, err := decodeMemorySection(rawSection.Contents)
			if err != nil {
				return nil, fmt.Errorf("error decoding memory section: %w", err)
			}
			module.Memory = memorySection
		case 6:
			globalSection, err := decodeGlobalSection(rawSection.Contents)
			if err != nil {
				return nil, fmt.Errorf("error decoding global section: %w", err)
			}
			module.Global = globalSection
		case 7:
			exportSection, err := decodeExportSection(rawSection.Contents)
			if err != nil {
				return nil, fmt.Errorf("error decoding export section: %w", err)
			}
			module.Export = exportSection
		case 8:
			startSection, err := decodeStartSection(rawSection.Contents)
			if err != nil {
				return nil, fmt.Errorf("error decoding start section: %w", err)
			}
			module.Start = startSection
		case 10:
			codeSection, err := decodeCodeSection(rawSection.Contents)
			if err != nil {
				return nil, fmt.Errorf("error decoding code section: %w", err)
			}
			module.Code = codeSection

			// default:
			// 	return nil, errors.New("unknown section type")
		}
	}

	if module.Function.Len() != module.Code.Len() {
		return nil, errors.New("function and code length must match")
	}

	return &module, nil
}

func decodeTypeSection(contents []byte) (*wasm.TypeSection, error) {
	reader := bytes.NewReader(contents)

	length, err := readLength(reader)
	if err != nil {
		return nil, err
	}

	functionTypes := make([]wasm.FunctionType, length)

	for i := range length {
		err := expectByte(reader, 0x60)
		if err != nil {
			return nil, err
		}

		paramsLength, err := readLength(reader)
		if err != nil {
			return nil, err
		}

		parameters := make([]wasm.ValType, paramsLength)

		for j := range paramsLength {
			valType, err := readValueType(reader)
			if err != nil {
				return nil, err
			}
			parameters[j] = valType
		}

		resultsLength, err := readLength(reader)
		if err != nil {
			return nil, err
		}

		results := make([]wasm.ValType, resultsLength)

		for j := range resultsLength {
			valType, err := readValueType(reader)
			if err != nil {
				return nil, err
			}
			results[j] = valType
		}

		functionTypes[i] = wasm.FunctionType{
			Parameters: parameters,
			Results:    results,
		}
	}

	if reader.Len() != 0 {
		return nil, UnderfullSection
	}

	return &wasm.TypeSection{FunctionTypes: functionTypes}, nil
}

func decodeImportSection(contents []byte) (*wasm.ImportSection, error) {
	reader := bytes.NewReader(contents)

	length, err := readLength(reader)
	if err != nil {
		return nil, err
	}

	imports := make([]wasm.Import, length)

	for i := range length {
		module, err := readName(reader)
		if err != nil {
			return nil, err
		}
		imports[i].Module = module

		name, err := readName(reader)
		if err != nil {
			return nil, err
		}
		imports[i].Name = name

		kind, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		extKind := wasm.ExternKind(kind)
		if !extKind.IsValid() {
			return nil, errors.New("invalid import kind")
		}

		imports[i].Desc.Kind = extKind

		switch extKind {
		case wasm.FuncKind:
			idx, err := ReadLEB128U32(reader)
			if err != nil {
				return nil, err
			}
			imports[i].Desc.FuncTypeIdx = idx
		case wasm.TableKind:
			limits, err := readTableType(reader)
			if err != nil {
				return nil, err
			}
			imports[i].Desc.TableLimits = limits
		case wasm.MemKind:
			limits, err := readLimits(reader)
			if err != nil {
				return nil, err
			}
			imports[i].Desc.MemLimits = limits
		case wasm.GlobalKind:
			globalType, err := readGlobalType(reader)
			if err != nil {
				return nil, err
			}
			imports[i].Desc.GlobalType = globalType
		}
	}

	if reader.Len() > 0 {
		return nil, UnderfullSection
	}

	return &wasm.ImportSection{Imports: imports}, nil
}

func decodeFunctionSection(contents []byte) (*wasm.FunctionSection, error) {
	reader := bytes.NewReader(contents)

	funcTypeIdxs, err := readIndexVector(reader)
	if err != nil {
		return nil, err
	}

	if reader.Len() > 0 {
		return nil, UnderfullSection
	}

	return &wasm.FunctionSection{FuncTypeIdxs: funcTypeIdxs}, nil
}

func decodeTableSection(contents []byte) (*wasm.TableSection, error) {
	reader := bytes.NewReader(contents)

	length, err := readLength(reader)
	if err != nil {
		return nil, err
	}

	tables := make([]wasm.Limits, length)

	for i := range length {
		limits, err := readTableType(reader)
		if err != nil {
			return nil, err
		}
		tables[i] = limits
	}

	if reader.Len() > 0 {
		return nil, UnderfullSection
	}

	return &wasm.TableSection{Tables: tables}, nil
}

func decodeMemorySection(contents []byte) (*wasm.MemorySection, error) {
	reader := bytes.NewReader(contents)

	length, err := readLength(reader)
	if err != nil {
		return nil, err
	}

	memories := make([]wasm.Limits, length)

	for i := range length {
		limits, err := readLimits(reader)
		if err != nil {
			return nil, err
		}
		memories[i] = limits
	}

	if reader.Len() > 0 {
		return nil, UnderfullSection
	}

	return &wasm.MemorySection{Memories: memories}, nil
}

func decodeGlobalSection(contents []byte) (*wasm.GlobalSection, error) {
	reader := bytes.NewReader(contents)

	length, err := readLength(reader)
	if err != nil {
		return nil, err
	}

	globals := make([]wasm.GlobalDefinition, length)

	for i := range length {
		globalType, err := readGlobalType(reader)
		if err != nil {
			return nil, err
		}
		globals[i].Type = globalType

		init, err := readExpr(reader)
		if err != nil {
			return nil, err
		}
		globals[i].Init = init
	}

	if reader.Len() > 0 {
		return nil, UnderfullSection
	}

	return &wasm.GlobalSection{Definitions: globals}, nil
}

func decodeExportSection(contents []byte) (*wasm.ExportSection, error) {
	reader := bytes.NewReader(contents)

	length, err := readLength(reader)
	if err != nil {
		return nil, err
	}

	exports := make([]wasm.Export, length)

	for i := range length {
		name, err := readName(reader)
		if err != nil {
			return nil, err
		}
		exports[i].Name = name

		kind, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		extKind := wasm.ExternKind(kind)
		if !extKind.IsValid() {
			return nil, errors.New("invalid export kind")
		}

		exports[i].Kind = extKind

		idx, err := ReadLEB128U32(reader)
		if err != nil {
			return nil, err
		}
		exports[i].Index = idx
	}

	if reader.Len() > 0 {
		return nil, UnderfullSection
	}

	return &wasm.ExportSection{Exports: exports}, nil
}

func decodeStartSection(contents []byte) (*wasm.StartSection, error) {
	reader := bytes.NewReader(contents)

	idx, err := ReadLEB128U32(reader)
	if err != nil {
		return nil, err
	}

	if reader.Len() > 0 {
		return nil, UnderfullSection
	}

	return &wasm.StartSection{FuncIdx: idx}, nil
}

func decodeCodeSection(contents []byte) (*wasm.CodeSection, error) {
	reader := bytes.NewReader(contents)

	length, err := readLength(reader)
	if err != nil {
		return nil, err
	}

	codes := make([]wasm.Function, length)

	for i := range length {
		size, err := readLength(reader)
		if err != nil {
			return nil, err
		}

		buffer := make([]byte, size)
		n, err := reader.Read(buffer)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		if n != int(size) {
			return nil, NotEnoughBytes
		}

		funcReader := bytes.NewReader(buffer)

		localLength, err := readLength(funcReader)
		if err != nil {
			return nil, err
		}

		locals := make([]wasm.Local, localLength)

		for j := range localLength {
			count, err := ReadLEB128U32(funcReader)
			if err != nil {
				return nil, err
			}
			locals[j].Count = count

			valType, err := readValueType(funcReader)
			if err != nil {
				return nil, err
			}
			locals[j].Type = valType
		}

		codes[i].Locals = locals

		expr, err := readExpr(funcReader)
		if err != nil {
			return nil, err
		}
		codes[i].Body = expr

		if funcReader.Len() > 0 {
			return nil, UnderfullSection
		}
	}

	var localCount int64
	for _, code := range codes {
		for _, local := range code.Locals {
			localCount += int64(local.Count)
		}
	}

	if localCount > 0xffffffff {
		return nil, errors.New("too many locals")
	}

	if reader.Len() > 0 {
		return nil, UnderfullSection
	}

	return &wasm.CodeSection{Codes: codes}, nil
}
