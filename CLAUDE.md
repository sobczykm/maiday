# Maiday — kontekst projektu

> Stan na 20.10.2026. Dla Claude Code: decyzje, struktura repo i to, czego nie widać w kodzie.

## Cel

- Wdrożyć **Maiday** (planer: dashboard, kalendarz, nawyki, notatnik, biblioteka, job-hunting), zaprojektowany przez Julię (UX/UI) w Claude, jako **publiczną** aplikację z kontami Google.
- GitOps, wszystko w kontenerach, niskie koszty, możliwość skalowania.
- Projekt do nauki Kubernetesa (właściciel: Mateusz, backend/DevOps). Na klastrze mają później działać kolejne aplikacje.
- Preferencje: minimalne, bezpośrednie rozwiązania; komunikacja po polsku; nic klikanego ręcznie na produkcji.

## Struktura repo (monorepo)

```
app/web/          frontend: index.html (cała aplikacja), auth-bg.webp, Dockerfile (nginx-unprivileged), nginx.conf
app/api/          (do zrobienia) API w Go
infra/bootstrap/  jednorazowo ręcznie: Argo CD + root Application  →  kubectl apply -k infra/bootstrap --server-side
                  (Argo nie zainstaluje sam siebie; potem zarządza tym katalogiem przez Application "argocd")
infra/argocd/     Applications zarządzane przez root (app of apps): argocd (self-managed), traefik, metrics-server, sealed-secrets, cnpg (operator), ApplicationSet "apps"
infra/apps/<app>/ manifesty aplikacji (Kustomize); ApplicationSet tworzy z katalogu Application w namespace o tej samej nazwie
.github/workflows/web.yml  build obrazu ghcr.io/sobczykm/maiday-web (linux/amd64) + commit nowego taga do infra/apps/maiday/kustomization.yaml
```

- Repo GitHub: `github.com/sobczykm/maiday`, publiczne (Argo CD nie potrzebuje poświadczeń; w Gicie tylko SealedSecrets, nigdy jawne sekrety).
- Pakiet GHCR `maiday-web` musi być publiczny (albo imagePullSecret).
- Wersje przypięte: Argo CD v3.4.11, chart Traefik 41.7.1, nginx-unprivileged 1.30.6-alpine.

## Ustalenia

- **Backend:** własne API w **Go** + Postgres przez **CloudNativePG**. Klaster Postgresa per aplikacja w jej namespace (`infra/apps/maiday/db.yaml`, `maiday-db`, 1 instancja, 5 Gi), bo sekret `maiday-db-app` generowany przez CNPG jest wtedy od razu dostępny dla API; wspólny klaster rozważyć przy kolejnych aplikacjach. Tabela `items(user_id, collection, id text, data jsonb, updated_at)`, klucz `(user_id, collection, id)`. Autoryzacja w API (filtr po `user_id`), bez Supabase/Firebase.
- **Auth:** tylko Google (OIDC w API, sesja w httpOnly cookie). E-mail/Apple z notatki Julii odrzucone na razie.
- **Sekrety:** Sealed Secrets.
- **Ingress:** Traefik (ingress-nginx jest wycofany). Jeden LoadBalancer na klaster.
- **Domena:** jeszcze brak. Na razie test po IP LoadBalancera (HTTP). HTTPS + cert-manager po zakupie domeny; do testów ewentualnie sslip.io. Google OAuth wymaga HTTPS i domeny publicznej.
- **Pliki:** S3-kompatybilny magazyn (docelowo Cloudflare R2; konta Cloudflare jeszcze nie ma).
- **Migracja z Claude:** import kopii JSON; pliki Julii nie są ważne, nie przenosimy ich.
- Kolejne komponenty (cert-manager, Sealed Secrets, CNPG, monitoring) dodajemy dopiero, gdy są potrzebne.

## Klaster

- DigitalOcean DOKS `fra1`, Kubernetes 1.37, kontekst `do-fra1-k8s-1-37-0-do-1-fra1-1792447752898`, HA control plane wyłączone, auto-upgrade wyłączony.
- Pula: **1 × s-2vcpu-4gb** (amd64, ~3 GB allocatable). Decyzja: zostajemy na jednym węźle (bez HA; brak miejsca na własny Prometheus — monitoring na zewnątrz). Każdy LoadBalancer i PVC kosztuje osobno.
- Traefik ma Service LoadBalancer (DO LB ~12 $/m-c), IP 134.199.190.18 — Maiday działa na http://134.199.190.18 (tryb localStorage).
- Lokalnie: Mac arm64 → obrazy budować dla `linux/amd64`. kubectl 1.34 jest za stary dla serwera 1.37; helm i gh niezainstalowane.

## Aplikacja (`app/web/index.html`)

- Jeden plik ~706 KB (w tym ~282 KB base64 tła logowania w CSS), bez frameworka i bez builda. Obok `auth-bg.webp` (1,45 MB) ładowany względnym URL-em z `.auth`.
- Zależności z CDN: Google Fonts, `lucide@1.48.0` z jsDelivr (bez SRI). Aplikacja działa bez nich (fallbacki).
- **Źródłem prawdy jest wersja Julii w Claude** — jej pushe nadpisują plik. Zmiany w `index.html` (adapter, logowanie) trzeba uzgadniać z nią albo trzymać poza plikiem.
- Zapis dziś: poza Claude localStorage (`maiday.v1`, cały stan jako jeden JSON); w Claude `window.claude.use('db'|'user'|'assets'|'downloads')` w kształcie Firestore (`db.doc(path).collection(c).doc(id).set/delete`, `onSnapshot` zwraca całą kolekcję).
- Punkty podmiany (szukaj po nazwie): `connect`, `enqueue`, `put`, `del`, `upload`, `dropAssets`, `blobUrl` (`'/_blob/' + id`), `downloadAsset`, `applyBackup` (woła `enqueue` bezpośrednio), `PROFILE`, `showAuth`/`hideAuth`/`#auth-google`, teksty o „wersji w Claude”, stopka „Zapisano w tej przeglądarce”.
- Opcja najmniej inwazyjna: adapter udający `window.claude.use()` + routing `/_blob/<id>` do API.
- `connect()` przy pierwszym połączeniu sam przenosi dane z localStorage na konto i czyści `maiday.v1`.
- Kod błędu `invalid_argument` z zapisu przełącza aplikację w tryb tylko do odczytu — API/adapter nie może go zwracać przypadkiem.

### Model danych

- 8 kolekcji per użytkownik: `tasks`, `habits`, `notes`, `folders`, `labels`, `days`, `library`, `jobs`. Każdy wpis to obiekt JSON z polem `id`; aplikacja ładuje całość i filtruje u siebie; zapis nadpisuje cały dokument (last write wins).
- `id` nie zawsze jest UUID (`labels`: np. `accent`; `days`: `YYYY-MM-DD`). Czasy w ms epoch, daty jako lokalne stringi — serwer niczego nie przelicza.
- Pliki do 20 MB (SVG 2 MB), ID musi pasować do `^[A-Za-z0-9_-]{8,64}$`. Odwołania w obiektach (`images`, `files`, `file`, `cv`, `cover`) i w HTML notatek (`<img data-asset>`, `sanitize()` buduje `src` z `blobUrl`).
- `blobUrl` jest synchroniczny i używany w `<img>`, CSS `url()`, linkach i `fetch` → pliki tylko same-origin z cookie (albo 302 na krótki presigned URL). Kopia zapasowa nie zawiera plików.

## Ryzyka do pamiętania

- SVG z tej samej domeny = XSS; pliki serwować z `nosniff` i `Content-Security-Policy: sandbox` (lub jako attachment).
- CSP wymaga `'unsafe-inline'` (inline script/style); na razie tylko Report-Only w `nginx.conf`.
- Aplikacja publiczna → RODO: regulamin i polityka prywatności (linki w ekranie logowania to dziś `#`), Google Fonts przekazują IP (rozważyć self-hosting fontów i Lucide).
- `dropAssets` jest best-effort → osierocone pliki w magazynie, przyda się GC.

## Etapy

1. **Fundament (zrobione 20.10.2026):** Argo CD (self-managed) + Traefik + metrics-server + frontend z obrazu GHCR, dostępny po IP LB.
2. **Backend v1:** API w Go, CNPG, Google OIDC, magazyn plików, Sealed Secrets, domena + cert-manager; adapter w froncie; backupy CNPG.
3. **Migracja:** import JSON z wersji w Claude.
4. **Rozwój:** monitoring, NetworkPolicy/quotas, preview z gałęzi `claude`, autoscaling, kolejne aplikacje.

## Otwarte decyzje

- Jak współpracujemy z Julią przy `index.html` (czy dodaje hooki/adapter u siebie).
- Domena; Cloudflare (DNS/Tunnel/R2) czy inny magazyn plików.
- Czy produkcja zostaje na DOKS, czy później k3s na VPS.
