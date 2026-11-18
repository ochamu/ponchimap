package d2render

import (
	"context"
	"image/color"
	"regexp"

	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2renderers/d2svgimport"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/imageasset"

	"github.com/ochamu/ponchimap/internal/render"
)

// D2 rejects a zero limit, so every field is set. The values are the ones
// the D2 CLI uses for its own PNG export. Scale is set for each figure.
var (
	assetLimits = imageasset.Limits{
		MaxFetchedBytes: 1 << 24, MaxEncodedBytes: 1 << 24, MaxDecompressedBytes: 1 << 24, MaxSVGBytes: 1 << 22,
		MaxDecodedWidth: 8192, MaxDecodedHeight: 8192, MaxDecodedPixels: 1 << 26, MaxAssets: 512,
		MaxCumulativeEncodedBytes: 1 << 28, MaxCumulativeDecodedBytes: 1 << 30,
	}
	svgImportLimits = d2svgimport.Limits{
		MaxBytes: 1 << 22, MaxDepth: 64, MaxElements: 20000, MaxAttributes: 100000, MaxAttributeBytes: 1 << 22,
		MaxPathCommands: 200000, MaxTransformFunctions: 20000, MaxUseDepth: 16, MaxResources: 2000,
	}
	svgImportBudget = d2scenebuild.SVGImportBudget{
		MaxSourceBytes: 1 << 26, MaxElements: 2000000, MaxAttributes: 10000000, MaxAttributeBytes: 1 << 28,
		MaxPathCommands: 20000000, MaxTransformFunctions: 2000000, MaxDeclaredResources: 200000, MaxExpandedUseInstances: 200000,
	}
	frameOptions = d2raster.FrameOptions{
		Background: color.White, MaxWidth: 32768, MaxHeight: 32768, MaxPixels: 64 << 20,
		MaxNodes: 1000000, MaxDepth: 1024,
		MaxPathCommands: 10000000, MaxTextRunesPerRun: 100000, MaxFontFacesPerText: 8,
		MaxTextCoverageChecks: 10000000, MaxTextShapingRuns: 100000,
		MaxAnimationTracks: 1000000, MaxAnimationKeyframes: 10000000,
		MaxAssets: 4096, MaxAssetBytes: 512 << 20, MaxDecodedAssetBytes: 512 << 20, MaxImportDepth: 1024,
		MaxOffscreenBytes: 512 << 20, MaxEvenOddClipWork: 250000000, MaxScanlineWork: 4000000000,
	}
)

// rasterize draws the compiled diagram to PNG in process, with no browser.
//
// D2 draws no frame that is over a limit of frameOptions. Which figures are
// over one is D2's to say: the limits on drawing work depend on what is in
// the figure. So a figure D2 does not draw at the full scale is asked for
// again at scale 1, and a figure it does not draw at scale 1 either has no
// PNG and scale 0. undrawn is D2's error for the last scale it did not draw.
func rasterize(ctx context.Context, diagram *d2target.Diagram, pad int64) (png []byte, scale int, undrawn, err error) {
	resolver, err := imageasset.New(imageasset.Options{Limits: assetLimits})
	if err != nil {
		return nil, 0, nil, err
	}
	doc, err := d2scenebuild.Build(ctx, diagram, d2scenebuild.Options{
		Pad:    &pad,
		Assets: &d2scenebuild.AssetOptions{Resolver: resolver, SVGImportLimits: svgImportLimits, SVGImportBudget: svgImportBudget},
	})
	if err != nil {
		return nil, 0, nil, err
	}
	options := frameOptions
	for _, s := range []int{render.FullPNGScale, 1} {
		options.Scale = float64(s)
		img, drawErr := d2raster.Render(ctx, doc, options)
		if ctx.Err() != nil {
			return nil, 0, nil, ctx.Err()
		}
		if drawErr != nil {
			undrawn = drawErr
			continue
		}
		png, err = d2raster.EncodePNG(ctx, img)
		if err != nil {
			return nil, 0, nil, err
		}
		return png, s, undrawn, nil
	}
	return nil, 0, undrawn, nil
}

// A limit error of D2's rasterizer is fixed words and numbers. Its other
// errors quote an ID or a text of the figure, so only an error that is this
// shape from end to end is passed on as text that is safe to show.
var limitError = regexp.MustCompile(`^d2raster: ([a-z -]+(?: [0-9]+)?(?: bytes)? exceeds? limit [0-9]+)$`)

// pngLimit is the limit that D2's error for an undrawn PNG names, or empty
// when the error is not one of D2's limit errors.
func pngLimit(undrawn error) string {
	if undrawn == nil {
		return ""
	}
	if m := limitError.FindStringSubmatch(undrawn.Error()); m != nil {
		return m[1]
	}
	return ""
}
