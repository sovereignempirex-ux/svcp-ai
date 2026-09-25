package completions

import (
	"github.com/svpc-ai/svpc/internal/tui/components/dialog"
)

// CompositeContextGroup combines multiple completion providers into one
type CompositeContextGroup struct {
	providers []dialog.CompletionProvider
}

func (cg *CompositeContextGroup) GetId() string {
	return "composite"
}

func (cg *CompositeContextGroup) GetEntry() dialog.CompletionItemI {
	// Return a generic entry for the composite group
	return dialog.NewCompletionItem(dialog.CompletionItem{
		Title: "All",
		Value: "all",
	})
}

func (cg *CompositeContextGroup) GetChildEntries(query string) ([]dialog.CompletionItemI, error) {
	var allItems []dialog.CompletionItemI
	for _, provider := range cg.providers {
		items, err := provider.GetChildEntries(query)
		if err != nil {
			return nil, err
		}
		allItems = append(allItems, items...)
	}
	return allItems, nil
}

func NewCompositeContextGroup(providers ...dialog.CompletionProvider) dialog.CompletionProvider {
	return &CompositeContextGroup{
		providers: providers,
	}
}
