package vulnreport

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

type ExportKernel struct {
	TemplateProvider ReportTemplateProvider
	renderers        map[string]ReportRenderer
}

func NewExportKernel(provider ReportTemplateProvider, renderers ...ReportRenderer) (*ExportKernel, error) {
	kernel := &ExportKernel{
		TemplateProvider: provider,
		renderers:        make(map[string]ReportRenderer),
	}
	for _, renderer := range renderers {
		if err := kernel.RegisterRenderer(renderer); err != nil {
			return nil, err
		}
	}
	return kernel, nil
}

func (k *ExportKernel) RegisterRenderer(renderer ReportRenderer) error {
	if renderer == nil {
		return utils.Errorf("renderer is nil")
	}
	format := strings.ToLower(strings.TrimSpace(renderer.Format()))
	if format == "" {
		return utils.Errorf("renderer format is empty")
	}
	if k.renderers == nil {
		k.renderers = make(map[string]ReportRenderer)
	}
	k.renderers[format] = renderer
	return nil
}

func (k *ExportKernel) Render(
	ctx context.Context,
	snapshot *VulnerabilityReportSnapshot,
	templateID string,
	format string,
) ([]byte, *RenderedMeta, error) {
	if snapshot == nil {
		return nil, nil, utils.Errorf("snapshot is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return nil, nil, utils.Errorf("format is empty")
	}
	renderer, ok := k.renderers[format]
	if !ok {
		return nil, nil, utils.Errorf("renderer not registered for format: %s", format)
	}

	templateID = strings.TrimSpace(firstNonEmpty(templateID, snapshot.TemplateID))
	var tpl *ReportTemplate
	if templateID != "" {
		if k.TemplateProvider == nil {
			return nil, nil, utils.Errorf("template provider is nil")
		}
		var err error
		tpl, err = k.TemplateProvider.Get(ctx, templateID)
		if err != nil {
			return nil, nil, err
		}
	}
	return renderer.Render(ctx, snapshot, tpl)
}
