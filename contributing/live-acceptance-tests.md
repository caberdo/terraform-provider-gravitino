# Live acceptance test cases (against a real Gravitino)

Deze tests draaien tegen een **echte** Gravitino-server (`apache/gravitino:1.3.0`) via podman
en starten nooit een eigen HTTP-mock. Ze zijn te herkennen aan het prefix `TestLiveAcc`.

## Draaien

```bash
make testacc-live                                       # alle live tests
make testacc-live-filter F=TestLiveAccMetalakeResource  # één test
```

**Garantie dat de response niet van een mock is** (`acceptance.LivePreCheck` in
`internal/acceptance/live.go`):

1. Test skipt als `GRAVITINO_URI` niet is gezet.
2. Test faalt als `GET /api/version` geen geldige Gravitino-versie retourneert (lege versie
   of afwijkend van `GRAVITINO_EXPECT_VERSION`).
3. De tests draaien binnen het podman-compose-netwerk; de enige bereikbare server op
   `http://gravitino:8090` is de echte container.

## Testgevallen

| Test (functie) | Resource / data source | Stappen tegen echte server | Belangrijkste asserts |
|---|---|---|---|
| `TestLiveAccMetalakeResource` | `gravitino_metalake` | create → update → import → (teardown: delete) | name/comment/properties; `audit.creator` == `gravitino_principal`; `audit.create_time` gezet; geen drift na refresh (properties = enkel geconfigureerde keys; `in-use` wordt door provider gefilterd) |
| `TestLiveAccCatalogResource`, `...UpdateProperties` | `gravitino_catalog` (+ metalake) | create (hive, dummy `metastore.uris`) → read → update properties → (teardown: delete met `force=true`) | name/type `relational`/`catalog_provider` `hive`/comment/id; `properties.metastore.uris`; server-toegevoegde props (`in-use`, `gravitino.bypass.*`) komen niet in state; `audit.creator` == principal |
| `TestLiveAccSchemaResourceProperties`, `...CommentReplaces` | `gravitino_schema` (+ metalake, catalog) | create → properties bijwerken → comment-wijziging forceert vervanging | alleen `setProperty`/`removeProperty` worden verstuurd; `name`/`comment` → replace |
| `TestLiveAccFilesetResourceProperties`, `...Rename` | `gravitino_fileset` (+ metalake, catalog, schema) | create → properties bijwerken (in-place) → rename (in-place) | server-only property `default-location-name` komt niet in state; rename stuurt `rename` en werkt de id bij |
| `TestLiveAccTagResource` | `gravitino_tag` (+ metalake) | create → read → (teardown: delete) | name/comment; `audit.creator` == principal; `audit.create_time` gezet |
| `TestLiveAccRoleResource` | `gravitino_role` (+ metalake, catalog) | create met één securable object → tweede object erbij via de privilege-override → (teardown: delete) | `securable_objects` bevat METALAKE/CATALOG; server serialiseert lowercase, state toont de canonieke uppercase; `securable_objects[].fullName` is **relatief** t.o.v. de metalake |
| `TestLiveAccPolicyResource` | `gravitino_policy` (+ metalake) | create → update (comment/enabled/supported_object_types/custom_rules) | `supported_object_types` wordt lowercase geserialiseerd en uppercase teruggegeven; `custom_rules`/`enabled` convergeren zonder drift |
| `TestLiveAccUserResource`, `TestLiveAccGroupResource` | `gravitino_user`, `gravitino_group` (+ metalake) | create → refresh → (teardown: delete) | name/id/audit; vereist autorisatie op de server |
| `TestLiveAccOwnerResource` | `gravitino_owner` (+ metalake, catalog, group) | create (owner van een catalog) → refresh | `object_full_name` relatief (`my_catalog`); `owner_type` uppercase in state ondanks lowercase serverrespons; id `metalake.CATALOG.my_catalog` |
| `TestLiveAccJobTemplateResource`, `...Spark`, `...InvalidCombination` | `gravitino_job_template` (+ metalake) | create (shell) → update (comment + arguments) → rename → spark-variant; plus een combinatie die config-validatie moet weigeren | exacte register-payload (`{jobTemplate:{…}}`), read-back voor audit, rename stuurt de oude naam en werkt de id bij |
| `TestLiveAccJobResource`, `...UnknownTemplate` | `gravitino_job` | create (run) → refresh → (teardown: cancel) | `job_id`/`status`/`audit`; id `metalake.job-…`; onbekende template geeft een duidelijke API-fout |
| `TestLiveAccModelResource` | `gravitino_model` (+ model-catalog) | create → rename/comment/properties-update → import | `latest_version` computed; rename in-place zonder inconsistent-result |
| `TestLiveAccModelVersionResource` | `gravitino_model_version` (+ model) | link → read → import | versienummer wordt door de server toegekend (Computed); `uris` als map |
| `TestLiveAccHealthDataSources` | `gravitino_health`, `gravitino_liveness`, `gravitino_readiness` | read | status == `"up"` (echte server gebruikt lowercase, anders dan veel mocks) |
| `TestLiveAccPrincipalDataSource` | `gravitino_principal` | read | name == `"anonymous"` (standaardprincipal van de echte server zonder auth) |
| `TestLiveAccMetalakeDataSources` | `gravitino_metalakes` (list), `gravitino_metalake` (get) + `gravitino_metalake` resource | create → get via datasource → list → (teardown: delete) | datasource name/comment gelijk aan aangemaakte metalake |

## Bekende server-eigenaardigheden rond jobs en templates

- `DELETE /metalakes/{m}/jobs/templates/{t}` geeft HTTP 409 `InUseException` zolang er
  job runs aan de template hangen, en een onbestaande template geeft HTTP 200
  `{"dropped":false}` (geen 404).
- Een job die in de wachtrij geannuleerd wordt, blijft `cancelling` zolang geen job
  executor de annulering oppakt; de template blijft daardoor in gebruik. De job-live-test
  beheert de template daarom buiten Terraform, en `gravitino_job`'s destroy wacht
  maximaal 10s (best effort) op een terminale status.
- `DELETE /metalakes/{m}/jobs/runs/{jobId}` bestaat niet (HTTP 405): destroy annuleert.

## Waarom dit echte bugs blootlegde

Deze suite vond bij eerste uitvoer twee afwijkingen tussen de aannames in de mocktests en het
echte servergedrag, die inmiddels zijn gefixt:

- **`gravitino_principal`**: `GET /api/authn/me` retourneert `principal` als string
  (`"anonymous"`), niet als object met `name`/`roles`. (fix: commit `5d48ad7`)
- **`gravitino_catalog`**: de server voegt zelf `in-use` en `gravitino.bypass.*` toe aan de
  catalog-properties; de resource zette die in state → drift
  `.properties: new element "in-use" has appeared`. (fix: commit `2505383`)

## Wat een standaard 1.3.0-server wél en niet ondersteunt

Gemeten tegen `apache/gravitino:1.3.0` (default configuratie, `auth=none`). Dit bepaalt welke
resources live te testen zijn en met welke catalog-provider:

| Catalog-provider | Ondersteunt | Ondersteunt **niet** |
|---|---|---|
| `fileset` (`type = "fileset"`) | catalogs, schemas, filesets, tags, policies, job templates/jobs, users/groups/roles/owners (met autorisatie) | tables, views, functions, partitions, statistics, topics |
| `model` (`type = "model"`, `properties = { uri = "file:/..." }`) | models en model versions | de rest |

Server-configuratie-eisen:

- **roles / owners / users / groups**: vereisen `gravitino.authorization.enable=true`; zonder die
  vlag antwoorden die endpoints HTTP 405 `UnsupportedOperationException` (foutcode `1006`).
  `docker-compose.yml` zet deze vlag daarom aan. Let op: users/groups hangen aan autorisatie, niet
  aan de backing-service.
- **idp_user / idp_group**: vereisen de `idp-basic` plugin plus `gravitino.authenticators = basic`
  (onverenigbaar met de default `simple`), `gravitino.server.rest.extensionPackages =
  org.apache.gravitino.idp.web.rest.feature` en een initieel admin-wachtwoord. Op een default
  server geven `/api/idp/*` een lege 404.
- **secrets**: de secrets-API bestaat pas vanaf Gravitino 1.4; op 1.3.0 geeft die route een 404.

## Case-gevoeligheid van enum-waarden (belangrijk)

Gravitino accepteert enum-waarden **case-insensitive op de input**, maar serialiseert ze **altijd
lowercase**. Gemeten voorbeelden:

```
POST /api/metalakes/authz_ml/roles   {"securableObjects":[{"type":"METALAKE",
     "privileges":[{"name":"CREATE_CATALOG","condition":"ALLOW"}]}]}   -> 200
GET  /api/metalakes/authz_ml/roles/r1                                   -> "type":"metalake",
     "name":"create_catalog", "condition":"allow"
PUT  /api/metalakes/authz_ml/owners/CATALOG/c1  {"name":"anonymous","type":"USER"} -> 200
GET  /api/metalakes/authz_ml/owners/CATALOG/c1  -> {"owner":{"name":"anonymous","type":"user"}}
POST /api/metalakes/{ml}/policies  content.supportedObjectTypes ["CATALOG"] -> ["catalog"]
```

Providers moeten de serverwaarde dus normaliseren naar de canonieke (uppercase) vorm voordat die
in state belandt; anders faalt elke apply met "Provider produced inconsistent result after apply".

Ook gemeten: het padsegment `metadataObjectType` is **uppercase enkelvoud** — `/objects/CATALOG/...`
geeft 200, `/objects/catalogs/...` geeft 400 `IllegalArgumentException`.

## Payloads verifiëren tegen een draaiende server

Snel controleren of een request-body door de echte server geaccepteerd wordt (dit ving meerdere
mock-vs-real afwijkingen):

```bash
podman compose up -d gravitino
curl -s http://localhost:8090/api/version
curl -s -X POST http://localhost:8090/api/metalakes -H 'Content-Type: application/json' \
  -d '{"name":"probe_ml"}'
curl -s -X POST http://localhost:8090/api/metalakes/probe_ml/jobs/templates \
  -H 'Content-Type: application/json' \
  -d '{"jobTemplate":{"name":"t","jobType":"shell","executable":"/bin/echo"}}'
```

Onbekende JSON-velden worden **niet** genegeerd: je krijgt
`UnrecognizedPropertyException: Unrecognized field "x" (class ...)`. Dat is precies waarom
mock-payloads uit de OpenAPI-spec moeten komen en niet uit de Go-structs.

## Buiten scope (server-level, geen backing-services)

- table / view / function / partition / statistics: vereisen een lakehouse-catalog
  (hive/iceberg); een `fileset`-catalog antwoordt `Catalog does not support table operations`.
  Hiervoor is in CI dus geen live dekking — wijzigingen daaraan blijven gebaseerd op de OpenAPI-spec
  en mock-tests.
- topic: vereist een Kafka-catalog.
- idp_user / idp_group: vereisen de hierboven genoemde `idp-basic`-configuratie.
