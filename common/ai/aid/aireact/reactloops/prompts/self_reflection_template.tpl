<|SELF_REFLECTION_TASK_{{.Nonce}}|>
# Self-Reflection Analysis Task

You are performing a CRITICAL self-reflection analysis on a recently executed action. This reflection will be saved to long-term memory and used to improve future decision-making.

## Action Execution Details

<|ACTION_DETAILS_{{.Nonce}}|>
**Action Type**: {{.ActionType}}
**Iteration**: {{.IterationNum}}
**Execution Time**: {{.ExecutionTime}}
**Result**: {{.ResultStatus}}
{{if .ErrorMessage}}**Error Message**: {{.ErrorMessage}}{{end}}
<|ACTION_DETAILS_END_{{.Nonce}}|>

{{if .EnvironmentalImpact}}<|ENVIRONMENTAL_IMPACT_{{.Nonce}}|>
## Environmental Impact Observed

**State Changes**: {{.EnvironmentalImpact.StateChanges}}
**Side Effects**: {{.EnvironmentalImpact.SideEffects}}
**Positive Effects**: {{.EnvironmentalImpact.PositiveEffects}}
**Negative Effects**: {{.EnvironmentalImpact.NegativeEffects}}
<|ENVIRONMENTAL_IMPACT_END_{{.Nonce}}|>{{end}}

{{if .RelevantMemories}}<|RELEVANT_MEMORIES_{{.Nonce}}|>
## Relevant Historical Memories

The following memories from past experiences are relevant to this action:

{{.RelevantMemories}}
<|RELEVANT_MEMORIES_END_{{.Nonce}}|>{{end}}

{{if .PreviousReflections}}<|PREVIOUS_REFLECTIONS_{{.Nonce}}|>
## Previous Reflections

Here are reflections from recent actions that may provide context:

{{.PreviousReflections}}
<|PREVIOUS_REFLECTIONS_END_{{.Nonce}}|>{{end}}

<|ANALYSIS_REQUIREMENTS_{{.Nonce}}|>
## Your Analysis Task

**CRITICAL IMPORTANCE**: This reflection will be saved to long-term memory and will influence all future decision-making. Be thorough, specific, and actionable.

Please provide a comprehensive reflection analysis addressing:

1. **Effectiveness Evaluation**: Was this action effective? Did it achieve its intended goal? What could have been done better?

2. **Impact Assessment**: What were the actual positive and negative impacts on the system state? How does this compare to expectations?

3. **Learning Insights**: What CRITICAL lessons can we learn from this execution? What patterns emerged? What should ALWAYS be remembered?

4. **Future Recommendations**: What MANDATORY recommendations do you have for similar actions in the future? What should we do differently? What should we repeat?

5. **Risk Assessment**: What risks were involved? How were they handled? What risks should be anticipated in similar situations?
<|ANALYSIS_REQUIREMENTS_END_{{.Nonce}}|>

<|OUTPUT_SCHEMA_{{.Nonce}}|>
## Output Format

You MUST respond using the following JSON schema:

```jsonschema
{{.Schema}}
```
<|OUTPUT_SCHEMA_END_{{.Nonce}}|>

**REMEMBER**: Use strong, clear language. This reflection will guide future actions. Be specific, actionable, and thorough.
<|SELF_REFLECTION_TASK_END_{{.Nonce}}|>

