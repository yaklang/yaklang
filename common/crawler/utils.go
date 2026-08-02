package crawler

import (
	"net/url"
	"strings"
)

var popularJavaScriptLibraryFiles = []string{"react", "vue", "angular", "jquery", "lodash", "bootstrap", "express", "d3", "moment", "axios", "three", "socket.io", "underscore", "ember", "backbone", "redux", "meteor", "next", "nuxt", "gatsby", "svelte", "preact", "material-ui", "ant-design", "bulma", "semantic-ui", "foundation", "tailwind", "styled-components", "apollo", "graphql", "mobx", "knockout", "mithril", "aurelia", "stimulus", "alpine", "inferno", "riot", "cypress", "rxjs", "zone", "hammerjs", "yarn", "npm", "webpack", "babel", "gulp", "grunt", "browserify", "rollup", "eslint", "prettier", "stylelint", "typescript", "coffeescript", "polymer", "lit-element", "lit-html", "stencil", "dojo", "extjs", "raphael", "paper", "fabric", "konva", "anime", "mojs", "velocity", "greensock", "scrollmagic", "popmotion", "lazy", "immutable", "ramda", "bacon", "bluebird", "q", "when", "leaflet", "openlayers", "mapbox-gl", "highcharts", "amcharts", "chart", "echarts", "zrender", "dimple", "c3", "dc", "nvd3", "plottable", "sigma", "vivagraphjs", "jointjs", "cytoscape", "vis", "gojs", "fabric", "paper", "color"}

func isPopularJSLibrary(libraryFileName string) bool {
	for _, lib := range popularJavaScriptLibraryFiles {
		if strings.Contains(lib, strings.ToLower(libraryFileName)) {
			return true
		}
	}
	return false
}

// stripURLFragment removes the client-side fragment from a URL before the
// crawler turns it into an HTTP request or uses it as a deduplication key.
// Fragments are never sent to an HTTP server, so treating them as part of the
// request URL creates duplicate requests and misleading status codes.
func stripURLFragment(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return raw
	}
	u.Fragment = ""
	u.RawFragment = ""
	if (strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https")) && u.Host != "" && u.Path == "" {
		u.Path = "/"
	}
	return u.String()
}
