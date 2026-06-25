package loop_ssa_api_discovery

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractMountPrefixesFromJava_AddPathPrefix(t *testing.T) {
	src := `public class AdminConfig implements WebMvcConfigurer {
    public void addInterceptors(InterceptorRegistry registry) {
        registry.addInterceptor(new AdminInterceptor()).addPathPatterns("/admin/**");
    }
    public void configurePathMatch(PathMatchConfigurer configurer) {
        configurer.addPathPrefix("/admin", c -> true);
    }
}`
	facts := extractMountPrefixesFromJava(src, "config/AdminConfig.java")
	require.NotEmpty(t, facts)
	prefixes := map[string]struct{}{}
	for _, f := range facts {
		prefixes[f.MountPrefix] = struct{}{}
	}
	require.Contains(t, prefixes, "/admin")
}

func TestValidateConfigStageMountRequired(t *testing.T) {
	batch := []WorklistSeedItem{{RelPath: "config/AdminConfig.java", Category: worklistCategoryRoutingConfig, Priority: 1}}
	out := &CodeReadingStageOutput{Stage: 1, ReadFilesCompleted: []string{"config/AdminConfig.java"}}
	require.Error(t, validateConfigStageMountRequired(1, out, batch))
	out.RoutingFacts = []RoutingFact{{Kind: "webmvc", MountPrefix: "/admin", Ref: "config/AdminConfig.java"}}
	require.NoError(t, validateConfigStageMountRequired(1, out, batch))
}
