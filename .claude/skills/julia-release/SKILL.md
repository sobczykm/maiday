---
name: julia-release
description: Bring a new version of Maiday from Julia (the HTML file she designs in Claude) into the repo, keep the API adaptations working, find what her changes need from the backend, and prepare a PR. Use when Mateusz shares a new index.html / maiday-*.html from Julia.
---

# Nowa wersja Maiday od Julii

Julia projektuje aplikację w Claude i przysyła cały plik HTML. Ten plik nigdy nie jest edytowany ręcznie:

- `app/web/upstream/index.html` to jej wersja 1:1;
- `app/web/adapt.py` nakłada na nią nasze zmiany (lokalne fonty i Lucide, API zamiast `window.claude`, logowanie Google, usuwanie konta, linki prawne) i zapisuje `app/web/public/index.html`;
- CI (`web.yml`) sprawdza `adapt.py --check`, więc `public/` nie może się rozjechać z `upstream/`.

Plik Julii ma aplikację dwa razy, najpierw po polsku, potem po angielsku. Dlatego większość zmian w `adapt.py` dotyczy dwóch miejsc.

## Kroki

1. **Gałąź.** `git switch -c julia/<data>` od aktualnego `main`.

2. **Diff od poprzedniej wersji.** Zanim nadpiszesz plik, zapisz różnice:
   `diff app/web/upstream/index.html <nowy plik> > <scratchpad>/julia.diff`.
   Przeczytaj je fragmentami (plik ma ~1 MB, w tym duży base64 tła logowania). To jest lista tego, co Julia zmieniła.

3. **Podmiana i adaptacja.** Skopiuj nowy plik do `app/web/upstream/index.html` i uruchom `python3 app/web/adapt.py`.
   - Jeśli skrypt zgłosi zmianę z inną liczbą wystąpień, znajdź w nowym pliku nowe miejsce i popraw tekst w `CHANGES`. Nigdy nie poprawiaj `public/index.html` ręcznie.
   - Jeśli Julia sama zrobiła coś, co dotąd robił adapter (np. podpięła linki prawne), usuń tę zmianę z `adapt.py`.

4. **Przegląd pod kątem backendu.** Przejdź przez diff i sprawdź każdą pozycję:

   | Co w diffie | Co zrobić |
   |---|---|
   | Nowa kolekcja w `COLLS` | Dopisz ją do `collections` w `app/api/items.go` i do metryki `maiday_items` |
   | Nowe `window.claude.use('…')` (np. `sample`, `downloads`) | Poza Claude ta funkcja nie działa. Zdecyduj razem z Mateuszem: zostawić fallback Julii, zrobić endpoint w API albo ukryć funkcję |
   | Nowe pola z plikami (`images`, `files`, `cv`, `cover`…) albo nowe typy plików w `EXT_TYPES` | `assets.go`: typy, limity; sprawdź, czy `blobUrl` i `dropAssets` nadal pokrywają wszystko |
   | Zmiana kształtu zapisanych danych (zmiana nazwy pola, inny format daty, nowe `tr`/`_lang`) | Czy stare wpisy w bazie nadal działają? Jeśli nie, potrzebna migracja (SQL na `items.data`). Limit 1 MB na wpis |
   | Nowy zasób zewnętrzny (`<script src>`, `<link>`, `fetch('https://…')`, fonty) | Hostuj u siebie (`public/vendor`, `public/fonts`) i dodaj podmianę w `adapt.py`; inaczej zablokuje go CSP z `nginx.conf` |
   | Nowe dane osobowe albo dane o zdrowiu, nowy dostawca | Aktualizacja `prywatnosc.html` i `en/privacy.html` |
   | Zmiany w ekranie logowania, profilu, menu, kopii zapasowej | Sprawdź, czy przyciski adaptera (Wyloguj, Usuń konto, linki) nadal są w dobrym miejscu |

   Zmiany backendowe zrób na tej samej gałęzi. Zmiany wymagające decyzji (np. nowa funkcja oparta na `window.claude`) wypisz i zapytaj.

5. **Weryfikacja.**
   - `python3 app/web/adapt.py --check`
   - Składnia każdego `<script>`: wytnij inline skrypty do plików w scratchpadzie i uruchom `node --check`.
   - `cd app/api && go vet ./... && go build ./...`, jeśli zmieniało się API.
   - Test w przeglądarce: serwuj `app/web/public` z prostym mockiem `/api/me`, `/api/items` i `PUT`/`DELETE` (Python `http.server`, port lokalny). Sprawdź: zalogowany widok, wczytane dane, zapis wysyła `PUT`, przełączenie PL↔EN, brak błędów w konsoli, ekran logowania przy 401.

6. **PR.** Commit i push robi Mateusz. Przygotuj opis PR w dwóch częściach: „Co zmieniła Julia” (po ludzku, z punktu widzenia użytkownika) oraz „Co dopisaliśmy” (adapt.py, backend, dokumenty, decyzje do podjęcia). Zaktualizuj `CLAUDE.md`, jeśli zmieniło się coś trwałego.

## Zasady

- Wpisy użytkownika zostają w języku, w którym je napisano. Przełączanie języka zmienia tylko interfejs, więc nie podłączamy tłumaczenia wpisów poza Claude.

- Nie zmieniaj wyglądu ani zachowania, które zaprojektowała Julia. Adaptacja tylko podłącza jej aplikację do naszej infrastruktury. Jeśli coś w jej wersji psuje się poza Claude, opisz to Mateuszowi zamiast przeprojektowywać.
- Teksty dodawane przez adapter piszemy po polsku i po angielsku (`TEXT` w `adapt.py`).
- Kod błędu `invalid_argument` przełącza aplikację w tryb tylko do odczytu, więc adapter nie może go zwracać.
