# Incident triage

Read `/app/incident.log` and `/app/schema.json`.

Determine the affected account, source IP, successful endpoint, and incident
classification. Write the result to `/app/result.json` using the supplied
schema.

Do not merely answer in chat. The JSON artifact is the deliverable. Preserve
the two most relevant log event IDs in the `evidence` array.

