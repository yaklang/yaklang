package antlr4nasl

func InitPluginGroup(engine *ScriptEngine) {
	engine.AddScriptIntoGroup(PluginGroupApache, "Web Servers")
}
