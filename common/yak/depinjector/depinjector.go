package depinjector

func DependencyInject() {
	injectAiTools()
}

func injectAiTools() {
	// aiSearchTools := searchtools.NewAiToolsSearchClient(buildinaitools.GetAllTools, &searchtools.AiToolsSearchClientConfig{
	// 	SearchType: "ai",
	// 	ChatToAiFunc: func(msg string) (io.Reader, error) {
	// 		return ai.ChatStream(msg)
	// 	},
	// })
	// omnisearch.RegisterSearchers(aiSearchTools)
}
