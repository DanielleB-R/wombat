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
				return nil, fmt.Errorf("error decoding input section: %w", err)
			}
			module.Import = importSection
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
			// default:
			// 	return nil, errors.New("unknown section type")
		}
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
			imports[i].Desc.FuncIdx = idx
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
			valType, err := readValueType(reader)
			if err != nil {
				return nil, err
			}
			imports[i].Desc.GlobalType.ValType = valType
			mutable, err := readBool(reader)
			if err != nil {
				return nil, err
			}
			imports[i].Desc.GlobalType.Mutable = mutable
		}
	}

	if reader.Len() > 0 {
		return nil, UnderfullSection
	}

	return &wasm.ImportSection{Imports: imports}, nil
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
