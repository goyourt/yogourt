package test

import (
	"errors"
	"fmt"
	"plugin"
	"testing"

	"github.com/goyourt/yogourt/compiler"
)

// fakeSymbolLookuper implements the { Lookup(string) (plugin.Symbol, error) }
// surface compiler.LoadSymbolFrom needs, without opening a real .so file.
type fakeSymbolLookuper struct {
	symbols map[string]plugin.Symbol
}

func (f fakeSymbolLookuper) Lookup(symbol string) (plugin.Symbol, error) {
	sym, ok := f.symbols[symbol]
	if !ok {
		return nil, fmt.Errorf("plugin: symbol %s not found", symbol)
	}
	return sym, nil
}

func TestLoadSymbolFromMissingSymbol(t *testing.T) {
	looker := fakeSymbolLookuper{symbols: map[string]plugin.Symbol{}}

	_, err := compiler.LoadSymbolFrom[string](looker, "Missing")
	if err == nil {
		t.Fatal("expected an error for a missing symbol")
	}
	// Any lookup failure must be identifiable as "symbol absent" so callers
	// can treat optional symbols (like Permissions) accordingly.
	if !errors.Is(err, compiler.ErrSymbolNotFound) {
		t.Errorf("expected the error to wrap ErrSymbolNotFound, got: %v", err)
	}
}

func TestLoadSymbolFromUnexpectedType(t *testing.T) {
	wrongType := 42
	looker := fakeSymbolLookuper{symbols: map[string]plugin.Symbol{
		"Callbacks": &wrongType,
	}}

	_, err := compiler.LoadSymbolFrom[map[string]string](looker, "Callbacks")
	if err == nil {
		t.Fatal("expected an error for a symbol of unexpected type")
	}
	if err.Error() == "" {
		t.Error("expected a descriptive error message")
	}
	// A type mismatch means the symbol exists: it must stay distinguishable
	// from a missing symbol.
	if errors.Is(err, compiler.ErrSymbolNotFound) {
		t.Error("a type mismatch must not be reported as a missing symbol")
	}
}

func TestLoadSymbolFromNominal(t *testing.T) {
	value := map[string]string{"GET": "read"}
	looker := fakeSymbolLookuper{symbols: map[string]plugin.Symbol{
		"Permissions": &value,
	}}

	got, err := compiler.LoadSymbolFrom[map[string]string](looker, "Permissions")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if got == nil || (*got)["GET"] != "read" {
		t.Errorf("expected symbol value to be returned, got %v", got)
	}
}
