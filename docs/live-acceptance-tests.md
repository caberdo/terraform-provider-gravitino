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
| `TestLiveAccCatalogResource` | `gravitino_catalog` (+ metalake) | create (hive, dummy `metastore.uris`) → read → (teardown: delete met `force=true`) | name/type `relational`/`catalog_provider` `hive`/comment/id; `properties.metastore.uris`; server-toegevoegde props (`in-use`, `gravitino.bypass.*`) komen niet in state; `audit.creator` == principal |
| `TestLiveAccTagResource` | `gravitino_tag` (+ metalake) | create → read → (teardown: delete) | name/comment; `audit.creator` == principal; `audit.create_time` gezet |
| `TestLiveAccHealthDataSources` | `gravitino_health`, `gravitino_liveness`, `gravitino_readiness` | read | status == `"up"` (echte server gebruikt lowercase, anders dan veel mocks) |
| `TestLiveAccPrincipalDataSource` | `gravitino_principal` | read | name == `"anonymous"` (standaardprincipal van de echte server zonder auth) |
| `TestLiveAccMetalakeDataSources` | `gravitino_metalakes` (list), `gravitino_metalake` (get) + `gravitino_metalake` resource | create → get via datasource → list → (teardown: delete) | datasource name/comment gelijk aan aangemaakte metalake |

## Waarom dit echte bugs blootlegde

Deze suite vond bij eerste uitvoer twee afwijkingen tussen de aannames in de mocktests en het
echte servergedrag, die inmiddels zijn gefixt:

- **`gravitino_principal`**: `GET /api/authn/me` retourneert `principal` als string
  (`"anonymous"`), niet als object met `name`/`roles`. (fix: commit `5d48ad7`)
- **`gravitino_catalog`**: de server voegt zelf `in-use` en `gravitino.bypass.*` toe aan de
  catalog-properties; de resource zette die in state → drift
  `.properties: new element "in-use" has appeared`. (fix: commit `2505383`)

## Buiten scope (server-level, geen backing-services)

- user / group / role / owner / policy: vereisen `gravitino.authorization.enable=true` (server
  geeft anders foutcode `1006`).
- schema / table / fileset / topic / view en verdere hiërarchie: vereisen backing-containers
  (Hive metastore, Kafka, etc.).
