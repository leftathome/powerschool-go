# Test fixtures

## `assignments_lookup.json`

One captured response from `POST /ws/xte/assignment/lookup` against the
the reference PowerSchool instance — used by `TestGetAssignments_ParsesFixtureAndEnforcesHeaders`
in `../assignments_test.go`.

The fixture has been redacted:

- `$breach_mitigation` tokens stripped.
- Every `studentsdcid` set to `999999`.
- `sectionsdcid` and any other identifiers mapped to stable placeholders
  via `hack/redact` (see `hack/redact.config.example.json`).

Assignment names, scores, categories, due dates, and flag combinations
are the real response shape, so the test exercises the actual decoding
paths.

### Refreshing

The PowerSchool live data rotates every school year, so this fixture will
eventually drift from what the API currently returns. To regenerate:

1. Open a browser session against your PowerSchool instance and log in.
2. Visit a class's scores page.
3. Replay the `/ws/xte/assignment/lookup` POST with the body shape
   documented in `../docs/API_ENDPOINTS.md`; save the raw response.
4. Strip `$breach_mitigation` and zero out `studentsdcid`:

   ```sh
   jq '[.[] | del(.["$breach_mitigation"]) |
        ._assignmentsections |= map(
          ._assignmentscores |= map(.studentsdcid = 999999))]' \
       raw.json > tmp.json
   ```

5. Run the committed-placeholder redactor (requires a local
   `hack/redact.config.json` — see the example file):

   ```sh
   go run ./hack/redact -in tmp.json -out testdata/assignments_lookup.json
   ```

6. Re-run `go test ./...` and update test assertions if the new fixture's
   flag/category mix differs from the committed one.
