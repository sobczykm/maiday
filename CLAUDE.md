# Maiday — kontekst projektu

> Stan na 20.10.2026. Dla Claude Code: decyzje, struktura repo i to, czego nie widać w kodzie.

## Cel

- Wdrożyć **Maiday** (planer: dashboard, kalendarz, nawyki, notatnik, biblioteka, job-hunting), zaprojektowany przez Julię (UX/UI) w Claude, jako **publiczną** aplikację z kontami Google.
- GitOps, wszystko w kontenerach, niskie koszty, możliwość skalowania.
- Projekt do nauki Kubernetesa (właściciel: Mateusz, backend/DevOps). Na klastrze mają później działać kolejne aplikacje.
- Preferencje: minimalne, bezpośrednie rozwiązania; komunikacja po polsku; nic klikanego ręcznie na produkcji.

## Struktura repo (monorepo)

```
app/web/          frontend: Dockerfile (nginx-unprivileged), nginx.conf (cache, ścisła CSP); upstream/index.html = wersja Julii 1:1; adapt.py = nasze zmiany nakładane na nią; public/ = to, co serwuje nginx: index.html i demo.html (generowane przez adapt.py, nie edytować), auth-bg.webp, covers/ (grafiki okładek od Julii, `covers/<nazwa>.webp?v=2`), fonts/ (Google Fonts latin+latin-ext, self-hosted), vendor/lucide-1.48.0.min.js
app/api/          API w Go (stdlib net/http + pgx + x/oauth2), Dockerfile (distroless); schemat w schema.sql, migracja przy starcie
infra/bootstrap/  jednorazowo ręcznie: Argo CD + root Application  →  kubectl apply -k infra/bootstrap --server-side
                  (Argo nie zainstaluje sam siebie; potem zarządza tym katalogiem przez Application "argocd")
infra/argocd/     Applications zarządzane przez root (app of apps): argocd (self-managed), traefik, metrics-server, sealed-secrets, cnpg (operator), barman-cloud (plugin backupów CNPG), cert-manager, alloy + kube-state-metrics (monitoring), ApplicationSet "apps"
infra/platform/   konfiguracja komponentów platformy z Gita (np. cert-manager/cluster-issuer.yaml), dołączana jako drugie źródło Application
infra/apps/<app>/ manifesty aplikacji (Kustomize); ApplicationSet tworzy z katalogu Application w namespace o tej samej nazwie
.github/workflows/image.yml  wspólny: build obrazu ghcr.io/sobczykm/maiday-<name> (linux/amd64) + `kustomize edit set image` + commit (Argo wdraża)
.github/workflows/web.yml, api.yml  wywołują image.yml po zmianach w app/web / app/api (api najpierw go vet + build)
```

- Repo GitHub: `github.com/sobczykm/maiday`, publiczne (Argo CD nie potrzebuje poświadczeń; w Gicie tylko SealedSecrets, nigdy jawne sekrety).
- Pakiet GHCR `maiday-web` musi być publiczny (albo imagePullSecret).
- Wersje przypięte: Argo CD v3.4.11, chart Traefik 41.7.1, nginx-unprivileged 1.30.6-alpine.

## Ustalenia

- **Backend:** własne API w **Go** + Postgres przez **CloudNativePG**. Klaster Postgresa per aplikacja w jej namespace (`infra/apps/maiday/db.yaml`, `maiday-db`, 1 instancja, 5 Gi), bo sekret `maiday-db-app` generowany przez CNPG jest wtedy od razu dostępny dla API; wspólny klaster rozważyć przy kolejnych aplikacjach. Tabela `items(user_id, collection, id text, data jsonb, updated_at)`, klucz `(user_id, collection, id)`. Autoryzacja w API (filtr po `user_id`), bez Supabase/Firebase.
- **Auth:** tylko Google (OIDC w API, sesja w httpOnly cookie). E-mail/Apple z notatki Julii odrzucone na razie.
- **Sekrety:** Sealed Secrets.
- **Ingress:** Traefik (ingress-nginx jest wycofany). Jeden LoadBalancer na klaster.
- **Domena:** `maiday.julqaj.com`, demo: `demo.maiday.julqaj.com` (osobny Ingress `maiday-demo` z własnym certyfikatem `maiday-demo-tls`, tylko web; nginx po `$host` serwuje na `/` plik `demo.html`). DNS w OVH (ręcznie, poza GitOps): rekord A → IP LoadBalancera Traefika (demo: CNAME → `maiday.julqaj.com.`). TLS z Let's Encrypt przez cert-manager (ClusterIssuer `letsencrypt`, HTTP-01, bez e-maila), HTTP → HTTPS przez middleware Traefika `maiday/redirect-https`.
- **Pliki:** DigitalOcean Spaces, bucket `maiday-bckt` w `fra1` (prywatny, 5 $/m-c za konto). Klucze w SealedSecret `maiday-s3`.
- **Migracja z Claude:** import kopii JSON; pliki Julii nie są ważne, nie przenosimy ich.
- Kolejne komponenty (monitoring, magazyn plików) dodajemy dopiero, gdy są potrzebne.

## API (`app/api`)

- Endpointy: `GET /api/items` (wszystkie 10 kolekcji), `PUT|DELETE /api/items/{kolekcja}/{id}` (body = obiekt z tym samym `id`, max 1 MB), `POST /api/assets` (surowe body pliku, `Content-Type` z listy, limit 20 MB / SVG 2 MB, 1 GB na użytkownika), `DELETE /api/assets/{id}`, `GET /_blob/{id}` (tylko właściciel, stream z S3; `nosniff`, SVG z CSP `sandbox`), `GET /api/auth/login`, `GET /api/auth/callback`, `POST /api/auth/logout`, `GET /api/me`, `DELETE /api/me` (usuwa konto: pliki z prefiksu `<user_id>/` w buckecie, potem użytkownika z kaskadą), `GET /healthz`. Błędy jako `{"error": "..."}`.
- Tabele: `users` (id = Google `sub`, email, name — zdjęcia profilowego nie zapisujemy), `sessions` (sha256 tokena z cookie `maiday_session`, 30 dni), `items`, `assets` (metadane plików; obiekt w buckecie pod kluczem `<user_id>/<asset_id>`).
- Ochrona CSRF: cookie `SameSite=Lax` + `http.CrossOriginProtection`. `Secure` cookies, gdy `BASE_URL` zaczyna się od https.
- Env: `DATABASE_URL` (z sekretu CNPG `maiday-db-app`), `BASE_URL` (`https://maiday.julqaj.com`), `GOOGLE_CLIENT_ID`/`GOOGLE_CLIENT_SECRET` (SealedSecret `maiday-google`), `S3_ENDPOINT`/`S3_BUCKET` (+ opcjonalnie `S3_REGION`) i `S3_ACCESS_KEY_ID`/`S3_SECRET_ACCESS_KEY` (SealedSecret `maiday-s3`). Brak sekretu = funkcja wyłączona (503).
- Ingress `maiday` (`infra/apps/maiday/ingress.yaml`): host `maiday.julqaj.com`, `/api` → api, reszta → web. Google OAuth redirect URI: `https://maiday.julqaj.com/api/auth/callback`.

## Monitoring (Grafana Cloud)

- Stack `fondzinnia691` (region EU, `prod-eu-west-2`), plan Free (10k serii, 14 dni). Remote write: `https://prometheus-prod-65-prod-eu-west-2.grafana.net/api/prom/push`, instance ID 3690435.
- W klastrze tylko agent: Alloy (`infra/argocd/alloy.yaml`, Deployment w `monitoring`) + kube-state-metrics (`infra/argocd/kube-state-metrics.yaml`). Konfiguracja scrapowania w `infra/apps/monitoring/config.alloy` (ConfigMap `alloy-config`, przeładowanie automatyczne), token w SealedSecret `grafana-cloud` (`PROM_USERNAME`, `PROM_PASSWORD`). Każdy job ma listę dozwolonych metryk — nową metrykę trzeba dopisać do regexu `keep`. Walidacja: `alloy validate config.alloy`.
- Źródła: kubelet (`/metrics/resource`, `/metrics` — wolumeny), API Maiday `/metrics` (liczby produktowe, niepubliczne — Ingress nie kieruje `/metrics`), CNPG (port 9187, w tym `barman_cloud_*` o backupach), Traefik (port 9100), cert-manager (9402), kube-state-metrics.
- Do Grafana Cloud trafiają tylko metryki (sumy), bez logów i danych osobowych.
- Dashboardy jako kod: `infra/grafana/dashboards/*.json` (folder „Maiday”, datasource uid `grafanacloud-prom`), wysyłane przez `.github/workflows/grafana.yml` (sekret repo `GRAFANA_TOKEN` — service account `github-dashboards`, rola Editor). Zmiany robione w UI trzeba wyeksportować do JSON, inaczej następny push je nadpisze.
- Alerty: jeszcze nie skonfigurowane.

## Backupy bazy

- Plugin Barman Cloud (CNPG-I; wbudowany `barmanObjectStore` jest przestarzały). `infra/apps/maiday/backup.yaml`: `ObjectStore` `maiday-backups` → `s3://maiday-bckt/cnpg` (Spaces fra1, klucze z `maiday-s3`), ciągłe archiwum WAL + `ScheduledBackup` codziennie 03:00 UTC, retencja 30 dni. W `db.yaml` `spec.plugins` z `isWALArchiver: true`.
- Stan: `kubectl -n maiday get backups`, `kubectl -n maiday get cluster maiday-db -o jsonpath='{.status.conditions}'` (ContinuousArchiving).
- Odtworzenie: nowy `Cluster` z `bootstrap.recovery.source` → `externalClusters[].plugin` (`barman-cloud.cloudnative-pg.io`, `barmanObjectName: maiday-backups`, `serverName: maiday-db`). Restore przećwiczony 20.10.2026 (`infra/runbooks/restore-test.yaml`, osobny klaster bez `spec.plugins`): liczby wpisów zgodne z produkcją, odtworzenie ~2 min. RPO do ~5 min (WAL wysyłany najpóźniej co 5 min).
- Klucz prywatny Sealed Secrets trzymać poza klastrem — bez niego nie odszyfrujemy `maiday-s3` na nowym klastrze.

## Klaster

- DigitalOcean DOKS `fra1`, Kubernetes 1.37, kontekst `do-fra1-k8s-1-37-0-do-1-fra1-1792447752898`, HA control plane wyłączone, auto-upgrade wyłączony.
- Pula: **1 × s-2vcpu-4gb** (amd64, ~3 GB allocatable). Decyzja: zostajemy na jednym węźle (bez HA; brak miejsca na własny Prometheus — monitoring na zewnątrz). Każdy LoadBalancer i PVC kosztuje osobno.
- Traefik ma Service LoadBalancer (DO LB ~12 $/m-c), IP 134.199.190.18 (stałe, dopóki istnieje Service Traefika; usunięcie go zmieni IP i wymaga poprawki DNS w OVH).
- Lokalnie: Mac arm64 → obrazy budować dla `linux/amd64`. kubectl 1.34 jest za stary dla serwera 1.37; helm i gh niezainstalowane.

## Aplikacja (`app/web/public/index.html`)

- Jeden plik ~1,6 MB (obie wersje językowe, w tym ~282 KB base64 tła logowania w CSS), bez frameworka i bez builda. **Eksport Julii to sama treść artefaktu, bez `<!doctype>`/`<head>`/`charset`/`viewport`** — szkielet dodaje `adapt.py` (bez niego krzaki w polskich znakach, a loader języka nie znajduje `#markup-pl-start`, bo parser zostawia go w `<head>`). Obok `auth-bg.webp` (1,45 MB) ładowany względnym URL-em z `.auth`.
- Bez zależności zewnętrznych: fonty z `fonts/fonts.css`, Lucide z `vendor/` (sprawdzony shasum paczki npm). Wersja Julii z Claude ładuje je z Google Fonts / jsDelivr — przy jej nowych wersjach trzeba podmienić te dwa odwołania (razem z adapterem API).
- **Źródłem prawdy jest wersja Julii w Claude.** Jej plik trafia 1:1 do `app/web/upstream/index.html`, a `python3 app/web/adapt.py` generuje z niego `public/index.html`. CI (`web.yml`) sprawdza `adapt.py --check`. Nowe wersje od Julii wprowadza skill `/julia-release` (`.claude/skills/julia-release/`): diff, adaptacja, przegląd pod kątem backendu, test, PR.
- **Wersja demo** (od 24.10.2026): ta sama aplikacja z flagą Julii `TRIAL = true` (u niej plik `maiday-trial.html`) — bez logowania i API, ekran startowy „Zobacz wersję demo”, dane w `localStorage` pod kluczem `maiday.trial`, pliki w IndexedDB `maiday-trial-files`. `adapt.py` buduje ją z tego samego `upstream/index.html` (`DEMO_CHANGES`: flaga, klucz LS, tytuł) → `public/demo.html`; podmiany adaptera dotyczące konta (`logout`, usuwanie, przycisk Google, `dataReady`) mają warunek `|| TRIAL`. Wersja prezentacyjna Julii (`maiday-demo.html`, dane Alex Morgan) nie jest wdrażana.
- Od v4 (20.10.2026) aplikacja jest dwujęzyczna: plik zawiera ją dwa razy (`window.MAIDAY_APP.pl` i `.en`, EN generowany u Julii przez build.py), a loader przełącza język bez przeładowania. Dane: `R` (zapisane) i `S` (z nałożonymi tłumaczeniami), `setRaw()`; wpisy mają opcjonalne `tr` i `_lang`. Tłumaczenie wpisów (`window.claude.use('sample')`) działa tylko w Claude; na domenie `adapt.py` je wyłącza — decyzja: wpisy zostają w języku, w którym je napisano, przełączany jest tylko interfejs.
- **Adapter API (`adapt.py`, `CHANGES`, każda zmiana sprawdza liczbę wystąpień):** szkielet dokumentu; tło logowania `auth-bg.webp` zamiast `/_blob/<id>` z artefaktu; `LEGAL` (adresy dokumentów, Julia zostawia `'#'`) per język; `dataReady()` poza Claude = `apiLoaded` (migracje frontu czekają na dane z konta, inaczej działają na pustym stanie lokalnym). Przyciski „Wyloguj”/„Usuń konto” i linki prawne w Ustawieniach są już w wersji Julii — adapter tylko podpina `logout()` i `deleteAccountApi()`. `connect()` poza Claude woła `connectApi()`: `GET /api/me` (401 → ekran logowania `showAuth`), `PROFILE` z konta + `paintProfile()` (w Claude zostaje przykładowy profil Julii), `GET /api/items` przez `setRaw` do `R`/`S`, a `refs[c].doc(id).set/delete` mapowane na `PUT`/`DELETE /api/items/...` — dzięki temu `enqueue`, `put`, `del` i `applyBackup` działają bez zmian. Odświeżenie danych przy powrocie do karty (bez realtime). Pliki: `assetsApi` = `apiAssets` (`POST /api/assets`, `DELETE /api/assets/{id}`), `blobUrl()` bez zmian (`/_blob/<id>`). Przycisk Google → `/api/auth/login`, „Wyloguj” → `POST /api/auth/logout` + reload. Profil (desktop i menu mobilne): „Wyloguj”, „Usuń konto” (`confirmDialog` → `DELETE /api/me` → czyszczenie `maiday.*` w localStorage → reload), linki do Regulaminu i Prywatności (PL: `/regulamin`, `/prywatnosc`; EN: `/en/terms`, `/en/privacy`). Kody błędów z `api()`: `unavailable`, `unauthenticated`, `rejected` (nigdy `invalid_argument`).
- Zapis dziś: poza Claude localStorage (`maiday.v1`, cały stan jako jeden JSON); w Claude `window.claude.use('db'|'user'|'assets'|'downloads')` w kształcie Firestore (`db.doc(path).collection(c).doc(id).set/delete`, `onSnapshot` zwraca całą kolekcję).
- Punkty podmiany (szukaj po nazwie): `connect`, `enqueue`, `put`, `del`, `upload`, `dropAssets`, `blobUrl` (`'/_blob/' + id`), `downloadAsset`, `applyBackup` (woła `enqueue` bezpośrednio), `PROFILE`, `showAuth`/`hideAuth`/`#auth-google`, teksty o „wersji w Claude”, stopka „Zapisano w tej przeglądarce”.
- `connect()` przy pierwszym połączeniu sam przenosi dane z localStorage na konto i czyści `maiday.v1`.
- Kod błędu `invalid_argument` z zapisu przełącza aplikację w tryb tylko do odczytu — API/adapter nie może go zwracać przypadkiem.

### Model danych

- 10 kolekcji per użytkownik: `tasks`, `habits`, `notes`, `folders`, `labels`, `days`, `library`, `jobs`, `pages`, `rows`. Od wersji z 23.10.2026 Biblioteka i Job-hunting to „własne zakładki”: `pages` (zakładki z modułami table/notes/todo/gallery + dokument `_nav` z kolejnością/ukryciem), `rows` (wpisy tabel, listy TO-DO `kind:'list'`, zdjęcia galerii `kind:'img'`). Front sam przenosi stare `library`/`jobs` do `rows` (stałe id `library`/`jobs`, `_nav.seeded`) i czyści stare kolekcje; zostają w API dla kont jeszcze niezmigrowanych. Model opisany przez Julię: `docs/julia/wlasne-zakladki-model-danych.md`. Usuwanie zakładki/modułu/kolumny (z plikami) robi front. Każdy wpis to obiekt JSON z polem `id`; aplikacja ładuje całość i filtruje u siebie; zapis nadpisuje cały dokument (last write wins).
- `id` nie zawsze jest UUID (`labels`: np. `accent`; `days`: `YYYY-MM-DD`). Czasy w ms epoch, daty jako lokalne stringi — serwer niczego nie przelicza.
- Pliki do 20 MB (SVG 2 MB), ID musi pasować do `^[A-Za-z0-9_-]{8,64}$`. Odwołania w obiektach (`images`, `files`, `file`, `cv`, `cover`) i w HTML notatek (`<img data-asset>`, `sanitize()` buduje `src` z `blobUrl`).
- `blobUrl` jest synchroniczny i używany w `<img>`, CSS `url()`, linkach i `fetch` → pliki tylko same-origin z cookie (albo 302 na krótki presigned URL). Kopia zapasowa nie zawiera plików.

## Ryzyka do pamiętania

- SVG z tej samej domeny = XSS: `/_blob` serwuje SVG z CSP `sandbox` i wszystko z `nosniff` (zrobione).
- CSP wymuszana w `nginx.conf`: tylko `'self'` (+ `'unsafe-inline'` dla script/style, `data:`/`blob:` dla obrazów). Nowy zewnętrzny zasób w `index.html` trzeba albo hostować u siebie, albo dopisać do CSP.
- RODO/dokumenty: administrator danych Mateusz Sobczyk (osoba fizyczna), kontakt maiday@sobczyk.dev. Strony statyczne w `public/`: `/prywatnosc`, `/regulamin`, `/en/privacy`, `/en/terms` (wspólny `legal.css`; nginx `try_files $uri.html`). Treść opisuje faktyczne działanie (dane z Google, cookies `maiday_session`/`maiday_oauth_state`, DigitalOcean fra1, backupy do ~30 dni, dni choroby = dane o zdrowiu, zgoda art. 9). Przy zmianie zbieranych danych, dostawców, limitów lub retencji — zaktualizować obie wersje językowe.
- Licencje: fonty OFL (`fonts/OFL-*.txt`), Lucide ISC (`vendor/lucide-LICENSE.txt`). Kod Maiday bez licencji (all rights reserved; projekt i logo Julii).
- Google OAuth consent screen: ustawić link do polityki prywatności i regulaminu, stronę główną i status „In production”.
- `dropAssets` jest best-effort → osierocone pliki w magazynie, przyda się GC.

## Etapy

1. **Fundament (zrobione 20.10.2026):** Argo CD (self-managed) + Traefik + metrics-server + frontend z obrazu GHCR, dostępny po IP LB.
2. **Backend v1:** API w Go, CNPG, Google OIDC, magazyn plików, Sealed Secrets, domena + cert-manager; adapter w froncie; backupy CNPG (zrobione).
3. **Migracja:** import JSON z wersji w Claude.
4. **Rozwój:** monitoring, NetworkPolicy/quotas, preview z gałęzi `claude`, autoscaling, kolejne aplikacje.

## Otwarte decyzje

- Współpraca z Julią: na razie przysyła plik, a Mateusz wprowadza go skillem `/julia-release`. Później ewentualnie wgrywanie przez nią do `app/web/upstream/` na gałęzi `julia` + preview.
- Czy produkcja zostaje na DOKS, czy później k3s na VPS.
