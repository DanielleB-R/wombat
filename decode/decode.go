package decode

import (
	"bytes"
	"fmt"

	"github.com/DanielleB-R/wombat/wasm"
)

// Decode decodes a WASM module from the contents of its file
func Decode(contents []byte) (*wasm.Module, error) {
	reader := bytes.NewReader(contents)

	sections, err := ReadSections(reader)
	if err != nil {
		return nil, err
	}

	return ParseSections(sections)
}

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
			return nil, ErrUnexpectedEOF
		}

		if id != 0 && id <= lastID {
			return nil, ErrSectionOutOfOrder
		}
		if id > 11 {
			return nil, ErrUnknownSectionType
		}
		if id != 0 {
			lastID = id
		}

		length, err := readLEB128U32(reader)
		if err != nil {
			return nil, fmt.Errorf("error reading section length: %w", err)
		}

		if length > uint32(reader.Len()) {
			return nil, ErrLengthTooLong
		}

		contents := make([]byte, length)
		_, _ = reader.Read(contents)

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

// ParseSections decodes the contents of the module into appropriate data structures
func ParseSections(raw *wasm.RawModule) (*wasm.Module, error) {
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
		case 9:
			elementSection, err := decodeElementSection(rawSection.Contents)
			if err != nil {
				return nil, fmt.Errorf("error decoding element section: %w", err)
			}
			module.Element = elementSection
		case 10:
			codeSection, err := decodeCodeSection(rawSection.Contents)
			if err != nil {
				return nil, fmt.Errorf("error decoding code section: %w", err)
			}
			module.Code = codeSection
		case 11:
			dataSection, err := decodeDataSection(rawSection.Contents)
			if err != nil {
				return nil, fmt.Errorf("error decoding data section: %w", err)
			}
			module.Data = dataSection

		default:
			return nil, ErrUnknownSectionType
		}
	}

	if module.Function.Len() != module.Code.Len() {
		return nil, ErrFunctionCodeMismatch
	}

	return &module, nil
}

func decodeTypeSection(contents []byte) (*wasm.TypeSection, error) {
	reader := bytes.NewReader(contents)

	functionTypes, err := readVec(reader, func(reader *bytes.Reader) (wasm.FunctionType, error) {
		var functionType wasm.FunctionType

		err := expectByte(reader, 0x60)
		if err != nil {
			return functionType, err
		}

		parameters, err := readVec(reader, readValueType)
		if err != nil {
			return functionType, err
		}

		results, err := readVec(reader, readValueType)
		if err != nil {
			return functionType, err
		}

		return wasm.FunctionType{
			Parameters: parameters,
			Results:    results,
		}, nil
	})

	if err != nil {
		return nil, err
	}

	if reader.Len() != 0 {
		return nil, ErrSectionMismatch
	}

	return &wasm.TypeSection{FunctionTypes: functionTypes}, nil
}

func decodeImportSection(contents []byte) (*wasm.ImportSection, error) {
	reader := bytes.NewReader(contents)

	imports, err := readVec(reader, func(reader *bytes.Reader) (wasm.Import, error) {
		var imp wasm.Import

		module, err := readName(reader)
		if err != nil {
			return imp, err
		}
		imp.Module = module

		name, err := readName(reader)
		if err != nil {
			return imp, err
		}
		imp.Name = name

		kind, err := reader.ReadByte()
		if err != nil {
			return imp, ErrUnexpectedEOF
		}
		extKind := wasm.ExternKind(kind)
		if !extKind.IsValid() {
			return imp, ErrBadEnum
		}

		imp.Desc.Kind = extKind

		switch extKind {
		case wasm.FuncKind:
			idx, err := readLEB128U32(reader)
			if err != nil {
				return imp, err
			}
			imp.Desc.FuncTypeIdx = idx
		case wasm.TableKind:
			limits, err := readTableType(reader)
			if err != nil {
				return imp, err
			}
			imp.Desc.TableLimits = limits
		case wasm.MemKind:
			limits, err := readLimits(reader)
			if err != nil {
				return imp, err
			}
			imp.Desc.MemLimits = limits
		case wasm.GlobalKind:
			globalType, err := readGlobalType(reader)
			if err != nil {
				return imp, err
			}
			imp.Desc.GlobalType = globalType
		}
		return imp, nil
	})

	if err != nil {
		return nil, err
	}

	if reader.Len() > 0 {
		return nil, ErrSectionMismatch
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
		return nil, ErrSectionMismatch
	}

	return &wasm.FunctionSection{FuncTypeIdxs: funcTypeIdxs}, nil
}

func decodeTableSection(contents []byte) (*wasm.TableSection, error) {
	reader := bytes.NewReader(contents)

	tables, err := readVec(reader, readTableType)
	if err != nil {
		return nil, err
	}

	if reader.Len() > 0 {
		return nil, ErrSectionMismatch
	}

	return &wasm.TableSection{Tables: tables}, nil
}

func decodeMemorySection(contents []byte) (*wasm.MemorySection, error) {
	reader := bytes.NewReader(contents)

	memories, err := readVec(reader, readLimits)
	if err != nil {
		return nil, err
	}

	if reader.Len() > 0 {
		return nil, ErrSectionMismatch
	}

	return &wasm.MemorySection{Memories: memories}, nil
}

func decodeGlobalSection(contents []byte) (*wasm.GlobalSection, error) {
	reader := bytes.NewReader(contents)

	globals, err := readVec(reader, func(reader *bytes.Reader) (wasm.GlobalDefinition, error) {
		var definition wasm.GlobalDefinition

		globalType, err := readGlobalType(reader)
		if err != nil {
			return definition, err
		}
		definition.Type = globalType

		init, err := readExpr(reader)
		if err != nil {
			return definition, err
		}
		definition.Init = init

		return definition, nil
	})

	if err != nil {
		return nil, err
	}

	if reader.Len() > 0 {
		return nil, ErrSectionMismatch
	}

	return &wasm.GlobalSection{Definitions: globals}, nil
}

func decodeExportSection(contents []byte) (*wasm.ExportSection, error) {
	reader := bytes.NewReader(contents)

	exports, err := readVec(reader, func(reader *bytes.Reader) (wasm.Export, error) {
		var export wasm.Export

		name, err := readName(reader)
		if err != nil {
			return export, err
		}
		export.Name = name

		kind, err := reader.ReadByte()
		if err != nil {
			return export, ErrUnexpectedEOF
		}
		extKind := wasm.ExternKind(kind)
		if !extKind.IsValid() {
			return export, ErrBadEnum
		}

		export.Kind = extKind

		idx, err := readLEB128U32(reader)
		if err != nil {
			return export, err
		}
		export.Index = idx

		return export, nil
	})

	if err != nil {
		return nil, err
	}

	if reader.Len() > 0 {
		return nil, ErrSectionMismatch
	}

	return &wasm.ExportSection{Exports: exports}, nil
}

func decodeStartSection(contents []byte) (*wasm.StartSection, error) {
	reader := bytes.NewReader(contents)

	idx, err := readLEB128U32(reader)
	if err != nil {
		return nil, err
	}

	if reader.Len() > 0 {
		return nil, ErrSectionMismatch
	}

	return &wasm.StartSection{FuncIdx: idx}, nil
}

func decodeElementSection(contents []byte) (*wasm.ElementSection, error) {
	reader := bytes.NewReader(contents)

	elements, err := readVec(reader, func(reader *bytes.Reader) (wasm.Element, error) {
		var element wasm.Element
		tableIdx, err := readLEB128U32(reader)
		if err != nil {
			return element, err
		}
		element.TableIdx = tableIdx

		offset, err := readExpr(reader)
		if err != nil {
			return element, err
		}
		element.Offset = offset

		init, err := readIndexVector(reader)
		if err != nil {
			return element, err
		}
		element.Init = init

		return element, nil
	})

	if err != nil {
		return nil, err
	}

	if reader.Len() > 0 {
		return nil, ErrSectionMismatch
	}

	return &wasm.ElementSection{Elements: elements}, nil
}

func decodeCodeSection(contents []byte) (*wasm.CodeSection, error) {
	reader := bytes.NewReader(contents)

	codes, err := readVec(reader, func(reader *bytes.Reader) (wasm.Function, error) {
		var function wasm.Function

		size, err := readLength(reader)
		if err != nil {
			return function, err
		}

		buffer := make([]byte, size)
		n, _ := reader.Read(buffer)
		if n != int(size) {
			return function, ErrUnexpectedEOF
		}

		funcReader := bytes.NewReader(buffer)

		locals, err := readVec(funcReader, func(reader *bytes.Reader) (wasm.Local, error) {
			var local wasm.Local

			count, err := readLEB128U32(funcReader)
			if err != nil {
				return local, err
			}
			local.Count = count

			valType, err := readValueType(funcReader)
			if err != nil {
				return local, err
			}
			local.Type = valType

			return local, nil
		})

		if err != nil {
			return function, err
		}

		var localCount int64

		for _, local := range locals {
			localCount += int64(local.Count)
		}

		if localCount > 0xffffffff {
			return function, ErrTooManyLocals
		}

		function.Locals = locals

		expr, err := readExpr(funcReader)
		if err != nil {
			return function, err
		}
		function.Body = expr

		if funcReader.Len() > 0 {
			return function, ErrSectionMismatch
		}

		return function, nil
	})

	if err != nil {
		return nil, err
	}

	if reader.Len() > 0 {
		return nil, ErrSectionMismatch
	}

	return &wasm.CodeSection{Codes: codes}, nil
}

func decodeDataSection(contents []byte) (*wasm.DataSection, error) {
	reader := bytes.NewReader(contents)

	entries, err := readVec(reader, func(reader *bytes.Reader) (wasm.DataEntry, error) {
		var entry wasm.DataEntry

		memIdx, err := readLEB128U32(reader)
		if err != nil {
			return entry, err
		}
		entry.MemIdx = memIdx

		offset, err := readExpr(reader)
		if err != nil {
			return entry, err
		}
		entry.Offset = offset

		initLength, err := readLength(reader)
		if err != nil {
			return entry, err
		}

		init := make([]byte, initLength)

		n, _ := reader.Read(init)
		if n != int(initLength) {
			return entry, ErrUnexpectedEOF
		}
		entry.Init = init

		return entry, nil
	})

	if err != nil {
		return nil, err
	}

	if reader.Len() > 0 {
		return nil, ErrSectionMismatch
	}

	return &wasm.DataSection{Entries: entries}, nil
}
