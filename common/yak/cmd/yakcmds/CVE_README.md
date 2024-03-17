# CVE Database how to?

## Pre

U should build cve database first, from nvd.

Use

```
yak build-cve-database
```

if u have done, check `default-cve.db` and `default-cve-description.db`

## AI Decoration

U can use `yak desc` command to decorate cve database.

## In CI

we maintain a cve database in CI (stored in aliyunoss).

```
# download
yak cve-download --ak ${{ secrets.OSS_KEY_ID }} --sk ${{ secrets.OSS_KEY_SECRET }}
# decoration (OPTIONAL)
yak desc --ai openai --apikey ${{ secrets.OPENAI_API_KEY }}
# merge description database into cve
yak cve-merge
yak cve-upload --ak ${{ secrets.OSS_KEY_ID }} --sk ${{ secrets.OSS_KEY_SECRET }}

```