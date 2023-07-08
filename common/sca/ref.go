package sca

/*
CDX-JSON

{
  "bomFormat": "CycloneDX",
  "specVersion": "1.3",
  "version": 1,
  "serialNumber": "urn:uuid:3e671687-395b-41f5-a30f-a58921a69b79",
  "components": [
    {
      "type": "library",
      "bom-ref": "pkg:maven/org.apache.commons/commons-lang3@3.12.0",
      "group": "org.apache.commons",
      "name": "commons-lang3",
      "version": "3.12.0",
      "purl": "pkg:maven/org.apache.commons/commons-lang3@3.12.0",
      "licenses": [
        {
          "license": {
            "id": "Apache-2.0",
            "url": "https://spdx.org/licenses/Apache-2.0"
          }
        }
      ]
    },
    {
      "type": "library",
      "bom-ref": "pkg:maven/com.fasterxml.jackson.core/jackson-databind@2.12.3",
      "group": "com.fasterxml.jackson.core",
      "name": "jackson-databind",
      "version": "2.12.3",
      "purl": "pkg:maven/com.fasterxml.jackson.core/jackson-databind@2.12.3",
      "licenses": [
        {
          "license": {
            "id": "Apache-2.0",
            "url": "https://spdx.org/licenses/Apache-2.0"
          }
        }
      ]
    }
  ]
}


SPDX-JSON:
{
  "spdxVersion": "SPDX-2.2",
  "dataLicense": "CC0-1.0",
  "SPDXID": "SPDXRef-DOCUMENT",
  "documentName": "Simple-Example",
  "documentNamespace": "http://spdx.org/spdxdocs/spdx-example-1",
  "creator": ["Tool: SPDX-Tools-2.2.0"],
  "created": "2023-07-08T00:00:00Z",
  "packages": [
    {
      "name": "Sample-Project",
      "versionInfo": "1.0.0",
      "downloadLocation": "git://github.com/spdx/sample-project.git",
      "licenseDeclared": "GPL-3.0-or-later",
      "licenseConcluded": "GPL-3.0-or-later",
      "copyrightText": "Copyright 2023 SPDX Example Corp.",
      "SPDXID": "SPDXRef-Package",
      "files": [
        {
          "name": "src/main.c",
          "checksums": [
            {
              "algorithm": "SHA1",
              "value": "2089455a4d9f0d6a5f5e2a3a1d252b4a5a5e4b6d"
            }
          ],
          "licenseConcluded": "GPL-3.0-or-later",
          "licenseInfoInFile": ["GPL-3.0-or-later"],
          "SPDXID": "SPDXRef-File-main.c"
        }
      ]
    }
  ],
  "relationships": [
    {
      "spdxElementId": "SPDXRef-DOCUMENT",
      "relationshipType": "DESCRIBES",
      "relatedSpdxElement": "SPDXRef-Package"
    },
    {
      "spdxElementId": "SPDXRef-Package",
      "relationshipType": "CONTAINS",
      "relatedSpdxElement": "SPDXRef-File-main.c"
    }
  ]
}
*/
