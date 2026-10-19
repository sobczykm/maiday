# Maiday — kontekst projektu

> Stan na 20.10.2026. Dla Claude Code: decyzje, struktura repo i to, czego nie widać w kodzie.

## Cel

- Wdrożyć **Maiday** (planer: dashboard, kalendarz, nawyki, notatnik, biblioteka, job-hunting), zaprojektowany przez Julię (UX/UI) w Claude, jako **publiczną** aplikację z kontami Google.
- GitOps, wszystko w kontenerach, niskie koszty, możliwość skalowania.
- Projekt do nauki Kubernetesa (właściciel: Mateusz, backend/DevOps). Na klastrze mają później działać kolejne aplikacje.
- Preferencje: minimalne, bezpośrednie rozwiązania; komunikacja po polsku; nic klikanego ręcznie na produkcji.

## Struktura repo (monorepo)

```
app/web/          frontend: Dockerfile (nginx-unprivileged), nginx.conf (cache, ścisła CSP); public/ = to, co serwuje nginx: index.html (cała aplikacja), auth-bg.webp, fonts/ (Google Fonts latin+latin-ext, self-hosted), vendor/lucide-1.48.0.min.js
app/api/          API w Go (stdlib net/http + pgx + x/oauth2), Dockerfile (distroless); schemat w schema.sql, migracja przy starcie
infra/bootstrap/  jednorazowo ręcznie: Argo CD + root Application  →  kubectl apply -k infra/bootstrap --server-side
                  (Argo nie zainstaluje sam siebie; potem zarządza tym katalogiem przez Application "argocd")
infra/argocd/     Applications zarządzane przez root (app of apps): argocd (self-managed), traefik, metrics-server, sealed-secrets, cnpg (operator), barman-cloud (plugin backupów CNPG), cert-manager, ApplicationSet "apps"
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
- **Domena:** `maiday.julqaj.com`. DNS w OVH (ręcznie, poza GitOps): rekord A → IP LoadBalancera Traefika. TLS z Let's Encrypt przez cert-manager (ClusterIssuer `letsencrypt`, HTTP-01, bez e-maila), HTTP → HTTPS przez middleware Traefika `maiday/redirect-https`.
- **Pliki:** DigitalOcean Spaces, bucket `maiday-bckt` w `fra1` (prywatny, 5 $/m-c za konto). Klucze w SealedSecret `maiday-s3`.
- **Migracja z Claude:** import kopii JSON; pliki Julii nie są ważne, nie przenosimy ich.
- Kolejne komponenty (monitoring, magazyn plików) dodajemy dopiero, gdy są potrzebne.

## API (`app/api`)

- Endpointy: `GET /api/items` (wszystkie 8 kolekcji), `PUT|DELETE /api/items/{kolekcja}/{id}` (body = obiekt z tym samym `id`, max 1 MB), `POST /api/assets` (surowe body pliku, `Content-Type` z listy, limit 20 MB / SVG 2 MB, 1 GB na użytkownika), `DELETE /api/assets/{id}`, `GET /_blob/{id}` (tylko właściciel, stream z S3; `nosniff`, SVG z CSP `sandbox`), `GET /api/auth/login`, `GET /api/auth/callback`, `POST /api/auth/logout`, `GET /api/me`, `GET /healthz`. Błędy jako `{"error": "..."}`.
- Tabele: `users` (id = Google `sub`), `sessions` (sha256 tokena z cookie `maiday_session`, 30 dni), `items`, `assets` (metadane plików; obiekt w buckecie pod kluczem `<user_id>/<asset_id>`).
- Ochrona CSRF: cookie `SameSite=Lax` + `http.CrossOriginProtection`. `Secure` cookies, gdy `BASE_URL` zaczyna się od https.
- Env: `DATABASE_URL` (z sekretu CNPG `maiday-db-app`), `BASE_URL` (`https://maiday.julqaj.com`), `GOOGLE_CLIENT_ID`/`GOOGLE_CLIENT_SECRET` (SealedSecret `maiday-google`), `S3_ENDPOINT`/`S3_BUCKET` (+ opcjonalnie `S3_REGION`) i `S3_ACCESS_KEY_ID`/`S3_SECRET_ACCESS_KEY` (SealedSecret `maiday-s3`). Brak sekretu = funkcja wyłączona (503).
- Ingress `maiday` (`infra/apps/maiday/ingress.yaml`): host `maiday.julqaj.com`, `/api` → api, reszta → web. Google OAuth redirect URI: `https://maiday.julqaj.com/api/auth/callback`.

## Backupy bazy

- Plugin Barman Cloud (CNPG-I; wbudowany `barmanObjectStore` jest przestarzały). `infra/apps/maiday/backup.yaml`: `ObjectStore` `maiday-backups` → `s3://maiday-bckt/cnpg` (Spaces fra1, klucze z `maiday-s3`), ciągłe archiwum WAL + `ScheduledBackup` codziennie 03:00 UTC, retencja 30 dni. W `db.yaml` `spec.plugins` z `isWALArchiver: true`.
- Stan: `kubectl -n maiday get backups`, `kubectl -n maiday get cluster maiday-db -o jsonpath='{.status.conditions}'` (ContinuousArchiving).
- Odtworzenie: nowy `Cluster` z `bootstrap.recovery.source` → `externalClusters[].plugin` (`barman-cloud.cloudnative-pg.io`, `barmanObjectName: maiday-backups`, `serverName: maiday-db`). Restore przećwiczyć na osobnym klastrze o innej nazwie, żeby nie nadpisać archiwum.
- Klucz prywatny Sealed Secrets trzymać poza klastrem — bez niego nie odszyfrujemy `maiday-s3` na nowym klastrze.

## Klaster

- DigitalOcean DOKS `fra1`, Kubernetes 1.37, kontekst `do-fra1-k8s-1-37-0-do-1-fra1-1792447752898`, HA control plane wyłączone, auto-upgrade wyłączony.
- Pula: **1 × s-2vcpu-4gb** (amd64, ~3 GB allocatable). Decyzja: zostajemy na jednym węźle (bez HA; brak miejsca na własny Prometheus — monitoring na zewnątrz). Każdy LoadBalancer i PVC kosztuje osobno.
- Traefik ma Service LoadBalancer (DO LB ~12 $/m-c), IP 134.199.190.18 (stałe, dopóki istnieje Service Traefika; usunięcie go zmieni IP i wymaga poprawki DNS w OVH).
- Lokalnie: Mac arm64 → obrazy budować dla `linux/amd64`. kubectl 1.34 jest za stary dla serwera 1.37; helm i gh niezainstalowane.

## Aplikacja (`app/web/public/index.html`)

- Jeden plik ~706 KB (w tym ~282 KB base64 tła logowania w CSS), bez frameworka i bez builda. Obok `auth-bg.webp` (1,45 MB) ładowany względnym URL-em z `.auth`.
- Bez zależności zewnętrznych: fonty z `fonts/fonts.css`, Lucide z `vendor/` (sprawdzony shasum paczki npm). Wersja Julii z Claude ładuje je z Google Fonts / jsDelivr — przy jej nowych wersjach trzeba podmienić te dwa odwołania (razem z adapterem API).
- **Źródłem prawdy jest wersja Julii w Claude.** Na razie `index.html` w repo ma nasz adapter API (patrz niżej), którego jej wersja nie zna — przy kolejnej wersji od Julii trzeba go przenieść albo dodać u źródła.
- **Adapter API (w `index.html`):** `connect()` poza Claude woła `connectApi()`: `GET /api/me` (401 → ekran logowania `showAuth`), `PROFILE` z konta + `paintProfile()`, `GET /api/items` do `S`, a `refs[c].doc(id).set/delete` mapowane na `PUT`/`DELETE /api/items/...` — dzięki temu `enqueue`, `put`, `del` i `applyBackup` działają bez zmian. Odświeżenie danych przy powrocie do karty (bez realtime). Pliki: `assetsApi` = `apiAssets` (`POST /api/assets`, `DELETE /api/assets/{id}`), `blobUrl()` bez zmian (`/_blob/<id>`). Przycisk Google → `/api/auth/login`, „Wyloguj” → `POST /api/auth/logout` + reload. Kody błędów z `api()`: `unavailable`, `unauthenticated`, `rejected` (nigdy `invalid_argument`).
- Zapis dziś: poza Claude localStorage (`maiday.v1`, cały stan jako jeden JSON); w Claude `window.claude.use('db'|'user'|'assets'|'downloads')` w kształcie Firestore (`db.doc(path).collection(c).doc(id).set/delete`, `onSnapshot` zwraca całą kolekcję).
- Punkty podmiany (szukaj po nazwie): `connect`, `enqueue`, `put`, `del`, `upload`, `dropAssets`, `blobUrl` (`'/_blob/' + id`), `downloadAsset`, `applyBackup` (woła `enqueue` bezpośrednio), `PROFILE`, `showAuth`/`hideAuth`/`#auth-google`, teksty o „wersji w Claude”, stopka „Zapisano w tej przeglądarce”.
- `connect()` przy pierwszym połączeniu sam przenosi dane z localStorage na konto i czyści `maiday.v1`.
- Kod błędu `invalid_argument` z zapisu przełącza aplikację w tryb tylko do odczytu — API/adapter nie może go zwracać przypadkiem.

### Model danych

- 8 kolekcji per użytkownik: `tasks`, `habits`, `notes`, `folders`, `labels`, `days`, `library`, `jobs`. Każdy wpis to obiekt JSON z polem `id`; aplikacja ładuje całość i filtruje u siebie; zapis nadpisuje cały dokument (last write wins).
- `id` nie zawsze jest UUID (`labels`: np. `accent`; `days`: `YYYY-MM-DD`). Czasy w ms epoch, daty jako lokalne stringi — serwer niczego nie przelicza.
- Pliki do 20 MB (SVG 2 MB), ID musi pasować do `^[A-Za-z0-9_-]{8,64}$`. Odwołania w obiektach (`images`, `files`, `file`, `cv`, `cover`) i w HTML notatek (`<img data-asset>`, `sanitize()` buduje `src` z `blobUrl`).
- `blobUrl` jest synchroniczny i używany w `<img>`, CSS `url()`, linkach i `fetch` → pliki tylko same-origin z cookie (albo 302 na krótki presigned URL). Kopia zapasowa nie zawiera plików.

## Ryzyka do pamiętania

- SVG z tej samej domeny = XSS: `/_blob` serwuje SVG z CSP `sandbox` i wszystko z `nosniff` (zrobione).
- CSP wymuszana w `nginx.conf`: tylko `'self'` (+ `'unsafe-inline'` dla script/style, `data:`/`blob:` dla obrazów). Nowy zewnętrzny zasób w `index.html` trzeba albo hostować u siebie, albo dopisać do CSP.
- Aplikacja publiczna → RODO: regulamin i polityka prywatności (linki w ekranie logowania to dziś `#`); brak funkcji „usuń konto”.
- `dropAssets` jest best-effort → osierocone pliki w magazynie, przyda się GC.

## Etapy

1. **Fundament (zrobione 20.10.2026):** Argo CD (self-managed) + Traefik + metrics-server + frontend z obrazu GHCR, dostępny po IP LB.
2. **Backend v1:** API w Go, CNPG, Google OIDC, magazyn plików, Sealed Secrets, domena + cert-manager; adapter w froncie; backupy CNPG (zrobione).
3. **Migracja:** import JSON z wersji w Claude.
4. **Rozwój:** monitoring, NetworkPolicy/quotas, preview z gałęzi `claude`, autoscaling, kolejne aplikacje.

## Otwarte decyzje

- Jak współpracujemy z Julią przy `index.html`. Pomysł: skill dla jej Claude, który po każdej zmianie wysyła nową wersję do repo (gałąź `claude` → preview, merge do `main`), bez odciągania jej uwagi od projektowania i z zachowaniem adaptera API.
- Czy produkcja zostaje na DOKS, czy później k3s na VPS.
