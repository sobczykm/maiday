# Własne zakładki: model danych (etapy 1–3 i galeria)

Front (wizualizacja) jest już w aplikacji Maiday. Ten dokument opisuje, co zapisuje front, żeby backend mógł przechowywać te same struktury. W podglądzie dane zapisują się tym samym mechanizmem co reszta aplikacji (kolekcje `pages`, `rows`, `notes` i `folders`).

## Biblioteka i Job-hunting są teraz własnymi zakładkami

Obie zakładki mają stałe id: `library` i `jobs`. W nawigacji i w adresie występują jako `p:library` i `p:jobs`. Stare adresy `#library` i `#jobs` przekierowują na nowe.

Przy pierwszym uruchomieniu front:

1. Tworzy obie zakładki, a w `_nav` zapisuje `seeded: true`.
2. Przenosi wpisy ze starych kolekcji `library` i `jobs` do `rows` z tymi samymi id i usuwa je ze starych kolekcji.

Migracja jest idempotentna, bo używa stałych id. Jeśli użytkownik usunie którąś z tych zakładek, nie wraca ona przy kolejnym uruchomieniu.

Mapowanie pól:

- **Biblioteka** (moduł `m-library`, kolumny `title`, `cat`, `tags`, `date`, `status`, `note`, `images`, `files`, `url`):
  - Ulubione są zapisane w `row.fav`.
  - Kategorie i statusy (także te edytowane przez użytkownika) stają się opcjami kolumn `cat` i `status`.
  - Status `none` zamienia się na `''`.
  - Stare pojedyncze pole `file` trafia do `files`.
- **Job-hunting** (moduł `m-jobs`, kolumny `company`, `role`, `desc`, `cv`, `posted`, `deadline`, `status`, `url`):
  - Wartość `cv` staje się jednoelementową tablicą plików.

Po migracji stare kolekcje `library` i `jobs` nie są już potrzebne. Wbudowane, nieusuwalne zakładki to: Dashboard, Kalendarz, Nawyki i Notatnik.

## Kolekcja `pages`: zakładki

```js
{
  id: string,
  name: string,                 // tytuł w nawigacji i na stronie (maks. 25 znaków ze spacjami)
  ico: 'lu:<Lucide name>',      // ikona, np. 'lu:BookOpen'
  btn: string,                  // etykieta głównego przycisku, np. 'Dodaj książkę'
  noBtn?: boolean,              // true = bez głównego przycisku na stronie (przełącznik „Dodatkowy przycisk” w konfiguratorze; nowa zakładka: true)
  btnMod?: string,              // id modułu, do którego „podpięty” jest główny przycisk
  cover: null | { type: 'color', value: 'sage'|'sky'|'sand'|'rust'|'rose'|'art-green'|'art-blue'|'art-wine'|'art-sand'|'art-shapes' }
         // art-* = grafiki z plików covers/*.webp (publikowane obok strony); stare wartości lilac/slate/dawn/dusk nadal się wyświetlają, ale nie ma ich już w wyborze
              | { type: 'image', id: <assetId> },
  modules: [ Tabela | Notatnik | ListyTODO | Galeria ],   // co najmniej 1; kolejność = kolejność na stronie
  created: number, updated: number
}
```

Zakładki nie da się zapisać bez żadnego modułu.

Dodatkowy przycisk strony (dawniej „główny przycisk”) jest opcjonalny. Jeśli jest włączony, wykonuje akcję modułu wskazanego w `btnMod`. Gdy `btnMod` jest puste albo wskazuje usunięty moduł, przycisk działa na pierwszym module na liście:

- tabela → okno nowej pozycji,
- listy TO-DO → nowa lista,
- notatnik → nowa notatka,
- galeria → wybór zdjęć.

### Moduł `table`

```js
{
  id, type: 'table', title,
  cols: [{
    id, name,
    type: 'text'|'note'|'number'|'date'|'time'|'status'|'cat'|'tags'|'link'|'check'|'fav'|'files'|'images',
          // 'title' = starszy typ (np. w zmigrowanych Bibliotece i Job-huntingu), traktowany jak 'text'
    opts?: [{ id, name, color? /* status */, ico? /* cat */ }]
  }],
  quick: [colId],               // maks. 3 pola w wierszu szybkiego dodawania pod tabelą
  opts?: {                      // widok tabeli
    search?: boolean,           // pole wyszukiwania nad tabelą
    filters?: boolean,          // filtry (wielokrotny wybór) dla kolumn status, cat i tags (tagi: wpis pasuje, gdy ma którykolwiek z wybranych); opcja widoczna tylko, gdy tabela ma taką kolumnę
    stats?: boolean             // liczba wpisów w każdym statusie w podtytule strony
  },
  tiles?: ['fav' | 'upcoming' | 'st:<optionId>']   // maks. 3 kafelki nad tabelą
}
```

Zasady:

- Kolumna typu `fav` (Ulubione) może być w tabeli tylko jedna. Pokazuje gwiazdkę, a wartość trzyma się w `row.fav`, nie w `row.v`. Jej obecność włącza filtr „Ulubione” i kafelek `fav`. Dawna opcja `opts.fav` jest migrowana przez front: tabela dostaje kolumnę `{ id: 'fav', name: 'Ulubione', type: 'fav' }` na początku, a `opts.fav` znika.
- Nazwa i typ każdej kolumny są obowiązkowe; konfigurator nie zapisze zakładki bez nich.
- Żadna kolumna nie jest stała: każdą można usunąć, przesunąć i zmienić jej typ. Tabela musi mieć co najmniej jedną kolumnę.
- Nowa tabela ma domyślnie jedną kolumnę: `{ name: 'Nazwa', type: 'text' }`.
- Nazwa wpisu (pogrubiony link w tabeli, tytuł okna szczegółów, etykieta ulubionych) to wartość kolumny typu `title`, a jeśli jej nie ma, pierwszej kolumny typu `text`. Gdy tabela nie ma kolumny tekstowej, wpis pokazuje się jako „Bez nazwy”. Cały wiersz i tak otwiera szczegóły.
- Konfigurator przy otwarciu edycji zamienia typ `title` na `text`, więc po zapisie zakładki starszy typ znika.
- Szybkie dodawanie nie wymaga już nazwy; wystarczy wypełnić dowolne pole.
- `quick` musi zawierać co najmniej 1 kolumnę, jeśli tabela ma kolumnę typu, który można dodać szybko (konfigurator nie zapisze zakładki bez tego; ostatniego zaznaczonego pola nie da się odznaczyć).
- Do `quick` mogą trafić tylko kolumny typu `title`, `text`, `note`, `number`, `date`, `time` (tylko godzina początku → `{ from }`), `status`, `cat`, `tags` i `link`.
- Każda data w kolumnie typu `date` trafia do Przypomnień na Dashboardzie: tydzień, 3 dni i 1 dzień wcześniej oraz w dniu terminu. Jeśli tabela ma też kolumnę typu `time`, jej godzina pokazuje się w opisie przypomnienia.

### Moduł `notes`: Notatnik, czyli foldery i notatki (maks. 1 na zakładkę)

```js
{ id, type: 'notes', title, folders: boolean }   // true = półka z folderami, false = jedna lista notatek; tytuł zawsze 'Notatnik' (nie można go zmienić)
```

- Notatki i foldery tego modułu to zwykłe rekordy z kolekcji `notes` i `folders`. Mają te same pola co w Notatniku i dodatkowe pole `scope: '<pageId>:<moduleId>'`.
- Notatnik pokazuje tylko rekordy bez `scope`. Każdy moduł pokazuje tylko rekordy ze swoim `scope`.
- Interfejs jest ten sam co w Notatniku: edytor, etykiety, okładki, ikony, zdjęcia i pliki.

### Moduł `todo`: listy TO-DO

```js
{ id, type: 'todo', title }   // kategorie list zostały usunięte: starsze pola useCats / cats front pomija i usuwa przy zapisie
```

Każda lista to rekord w `rows`:

```js
{ id, page, mod, kind: 'list', title, ord: number /* kolejność list: strzałki ‹ › albo przeciąganie całej listy za uchwyt */, color?: '#RRGGBB' | null /* kolor listy: jasne tło, kolor odhaczonych pozycji i paska postępu */,   // starsze listy mogą mieć jeszcze pole `cat` — jest ignorowane
  items: [{ id, text, done: boolean, /* pole imp (oznaczenie „ważne”) zostało usunięte — front go nie pokazuje ani nie zapisuje */ }],   // kolejność tablicy = kolejność na liście (drag-and-drop)
  created, updated }
```

- Listy wyświetlają się razem, bez podziału na kategorie.
- Układ: kolumny w stylu masonry (3 / 2 / 1 zależnie od szerokości). Lista o indeksie `i` (wg `ord`) trafia do kolumny `i % n`, więc nowa lista staje od razu pod poprzednią w tej kolumnie, bez pustych miejsc.

### Moduł `gallery`: galeria zdjęć i grafik

```js
{ id, type: 'gallery', title }
```

Każde zdjęcie to osobny rekord w `rows`:

```js
{ id, page, mod, kind: 'img', v: { images: [{ id: <assetId>, name }], url?: string /* opcjonalny link do zdjęcia, normalizowany do https://… */ }, created }
```

- Galeria przyjmuje pliki PNG, JPG, GIF i WebP, maks. 20 MB każdy. Można wgrać kilka naraz.
- Kolejność wyświetlania: od najnowszego (`created` malejąco).
- Kafelki są po 3 w rzędzie, na tablecie (do 900 px) po 2, a na telefonie (do 640 px) po 1. Nie zwijają się. Kliknięcie otwiera ten sam podgląd co w reszcie aplikacji, ze strzałkami między zdjęciami.
- Link dodaje się ikoną linku na zdjęciu (pole „Wklej link”). Zapisany link widać pod zdjęciem i można go usunąć.
- Usunięcie zdjęcia (krzyżyk na kafelku, z potwierdzeniem drugim kliknięciem) usuwa rekord i plik.

### Dokument specjalny `pages/_nav`: kolejność i widoczność zakładek

```js
{ id: '_nav', kind: 'nav', seeded: true, order: ['dashboard','calendar',…,'p:<pageId>'], hidden: ['p:jobs', …] }
```

- Ten dokument obejmuje też wbudowane zakładki.
- Dashboardu nie można ukryć.
- Własne zakładki można usuwać w Ustawieniach → Zakładki.

## Kolekcja `rows`: wpisy w tabelach

```js
{ id, page: <pageId>, mod: <moduleId>, fav?: boolean, v: { <colId>: value }, created, updated }
```

| typ kolumny | wartość |
|---|---|
| title, text, note, link | string (`link` jest normalizowany do `https://…`) |
| number | number lub null |
| check | boolean |
| fav | (brak w `v`) — `row.fav: boolean` |
| date | `'YYYY-MM-DD'` |
| time | `{ from?: 'HH:MM', to?: 'HH:MM' }` (jeśli są oba, `to` musi być późniejsze niż `from`) |
| status, cat | id opcji z `opts` (`''` oznacza brak) |
| tags | `string[]` (maks. 12) |
| files | `[{ id, name, size, type }]` |
| images | `[{ id, name }]` |

Wpisy tabel nie mają pola `kind`. Listy TO-DO mają `kind: 'list'`, a zdjęcia z galerii `kind: 'img'`.

## Usuwanie (robi to dziś front, backend powinien zrobić to samo)

- **Usunięcie zakładki** (w edycji zakładki albo w Ustawieniach → Zakładki): usuwa jej wpisy, listy i zdjęcia z `rows`, notatki i foldery z jej `scope` oraz wszystkie pliki z tych rekordów i zdjęcie tła. Zakładka znika też z `_nav`.
- **Usunięcie modułu w edycji zakładki:** usuwa jego wpisy, listy albo zdjęcia (razem z plikami), a w przypadku modułu `notes` jego notatki i foldery.
- **Usunięcie kolumny:** z każdego wpisu znika `v[colId]` razem z plikami.


## Własne kolory palety
- Zapisywane w kolekcji `labels` jako `{ id, kind: 'swatch', color: '#RRGGBB', created }`.
- Wspólne dla wszystkich okienek wyboru koloru (etykiety, statusy, listy). Użytkownik dodaje je przyciskiem „+” → wybór koloru → „Dodaj” i usuwa przez „Usuń kolory”. 12 kolorów domyślnej palety nie da się usunąć.
- Usunięcie koloru z palety nie zmienia etykiet, statusów ani list, które go już używają.
