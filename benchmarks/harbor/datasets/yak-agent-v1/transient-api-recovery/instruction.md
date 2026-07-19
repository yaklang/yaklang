# Recover a transient API result

Open `http://127.0.0.1:8080/` and follow the lab guidance. The token endpoint
has deterministic transient failures and does not require credentials.

Recover the token and write `/app/recovered.json` with:

```json
{"token": "...", "attempts": 0}
```

`attempts` must be the number of token requests you made. Do not stop after the
first temporary error, and do not merely return the token in chat.

