#!/usr/bin/env python3
"""Builds app/web/public/index.html from Julia's design (app/web/upstream/index.html).

Julia's file is the app as it runs inside Claude. This script makes the changes the app needs
on its own domain: fonts and Lucide served locally, data and files through the Maiday API,
Google sign-in, account deletion and links to the legal pages.

Her file contains the app twice (Polish, then English), so most changes are made once per language.
Every change names the exact text it replaces; if Julia's new version no longer contains that text
the expected number of times, the script stops and says which change has to be updated.

    python3 app/web/adapt.py           write public/index.html and public/demo.html
    python3 app/web/adapt.py --check   fail if they are not up to date
"""
import sys
from pathlib import Path

WEB = Path(__file__).resolve().parent
UPSTREAM = WEB / "upstream" / "index.html"
PUBLIC = WEB / "public" / "index.html"
DEMO = WEB / "public" / "demo.html"  # served at the root of demo.maiday.julqaj.com (nginx.conf)

TEXT = {
    "pl": {
        "profile": "Profil",
        "logout": "Wyloguj",
        "delete_failed": "Nie udało się usunąć konta. Spróbuj ponownie za chwilę.",
        "terms": ("Regulamin", "/regulamin"),
        "privacy": ("Prywatność", "/prywatnosc"),
        "connecting": "Łączenie…",
        "load_failed": "Nie udało się wczytać danych z konta. Odśwież stronę.",
        "saved": "Zapisano na Twoim koncie · widoczne tylko dla Ciebie",
        "session_expired": "Sesja wygasła. Zaloguj się ponownie, aby zapisać zmiany.",
    },
    "en": {
        "profile": "Profile",
        "logout": "Log out",
        "delete_failed": "Couldn’t delete the account. Try again in a moment.",
        "terms": ("Terms", "/en/terms"),
        "privacy": ("Privacy", "/en/privacy"),
        "connecting": "Connecting…",
        "load_failed": "Couldn’t load the data from your account. Refresh the page.",
        "saved": "Saved to your account · visible only to you",
        "session_expired": "Your session has expired. Sign in again to save your changes.",
    },
}

GOOGLE_G = '<svg viewBox="0 0 48 48" aria-hidden="true"><path fill="#EA4335" d="M24 9.5c3.54 0 6.71 1.22 9.21 3.6l6.85-6.85C35.9 2.38 30.47 0 24 0 14.62 0 6.51 5.38 2.56 13.22l7.98 6.19C12.43 13.72 17.74 9.5 24 9.5z"/><path fill="#4285F4" d="M46.98 24.55c0-1.57-.15-3.09-.38-4.55H24v9.02h12.94c-.58 2.96-2.26 5.48-4.78 7.18l7.73 6c4.51-4.18 7.09-10.36 7.09-17.65z"/><path fill="#FBBC05" d="M10.53 28.59c-.48-1.45-.76-2.99-.76-4.59s.27-3.14.76-4.59l-7.98-6.19C.92 16.46 0 20.12 0 24c0 3.88.92 7.54 2.56 10.78l7.97-6.19z"/><path fill="#34A853" d="M24 48c6.48 0 11.93-2.13 15.89-5.81l-7.73-6c-2.15 1.45-4.92 2.3-8.16 2.3-6.26 0-11.57-4.22-13.47-9.91l-7.98 6.19C6.51 42.62 14.62 48 24 48z"/></svg>'


def api_client(t):
    return f"""
  // ---------- Maiday API (the app on its own domain) ----------
  async function api(method, path, body) {{
    let res;
    try {{
      res = await fetch(path, {{ method, credentials: 'same-origin', headers: body ? {{ 'Content-Type': 'application/json' }} : {{}}, body: body ? JSON.stringify(body) : undefined }});
    }} catch (e) {{ throw {{ code: 'unavailable' }}; }}
    if (res.status === 401) throw {{ code: 'unauthenticated' }};
    if (res.status >= 500) throw {{ code: 'unavailable' }};
    if (!res.ok) throw {{ code: 'rejected', status: res.status }};
    return res.status === 204 ? null : res.json();
  }}
  const itemPath = (c, id) => `/api/items/${{c}}/${{encodeURIComponent(id)}}`;
  const apiCollection = c => ({{ doc: id => ({{ set: v => api('PUT', itemPath(c, id), v), delete: () => api('DELETE', itemPath(c, id)) }}) }});
  let apiLoaded = false; // the app migrates its data (dataReady) only once the account's data is here
  async function loadFromApi() {{
    const data = await api('GET', '/api/items');
    for (const c of COLLS) {{ R[c] = new Map(); S[c] = new Map(); for (const x of data[c] || []) setRaw(c, x); }}
    apiLoaded = true;
  }}
  const writesInFlight = () => [...queue.values()].some(q => q.running);
  const UPLOAD_ERRORS = {{ 401: 'unauthenticated', 411: 'unsupported_type', 413: 'too_large', 415: 'unsupported_type', 507: 'quota_or_state' }};
  const apiAssets = {{
    async upload(file, {{ type }}) {{
      let res;
      try {{ res = await fetch('/api/assets', {{ method: 'POST', credentials: 'same-origin', headers: {{ 'Content-Type': type }}, body: file }}); }}
      catch (e) {{ throw {{ code: 'store_unavailable' }}; }}
      if (!res.ok) throw {{ code: UPLOAD_ERRORS[res.status] || 'store_unavailable' }};
      return res.json();
    }},
    delete: id => api('DELETE', '/api/assets/' + encodeURIComponent(id))
  }};
  async function connectApi() {{
    let me;
    try {{ me = await api('GET', '/api/me'); }}
    catch (e) {{ if (e.code === 'unauthenticated') showAuth(); return; }}
    if (disposed) return;
    Object.assign(PROFILE, {{ name: me.name || me.email, email: me.email }});
    paintProfile();
    assetsApi = apiAssets;
    updateUploadUI();
    const pending = {{}}; for (const c of COLLS) pending[c] = [...R[c].values()];
    refs = {{}}; for (const c of COLLS) refs[c] = apiCollection(c);
    mode = 'db';
    setSync('', '{t["connecting"]}');
    try {{ await loadFromApi(); }}
    catch (e) {{ setSync('err', '{t["load_failed"]}'); return; }}
    if (disposed) return;
    // carry anything saved in this browser into the account once
    for (const c of COLLS) for (const x of pending[c]) if (!R[c].has(x.id)) {{ setRaw(c, x); enqueue(c, x.id, x); }}
    try {{ localStorage.removeItem(LS); }} catch (e) {{}}
    render();
    setSync('ok', '{t["saved"]}');

    document.addEventListener('visibilitychange', async () => {{
      if (disposed || document.visibilityState !== 'visible' || writesInFlight()) return;
      try {{ await loadFromApi(); render(); }} catch (e) {{}}
    }});
  }}
"""


def account_actions(t):
    return f"""  async function deleteAccountApi() {{
    try {{ await api('DELETE', '/api/me'); }}
    catch (e) {{ toast('{t["delete_failed"]}', 5000); return; }}
    try {{ Object.keys(localStorage).filter(k => k.startsWith('maiday.')).forEach(k => localStorage.removeItem(k)); }} catch (e) {{}}
    location.reload();
  }}
  async function logout() {{
    if (inClaude() || TRIAL) {{ showAuth(); return; }}
    try {{ await api('POST', '/api/auth/logout'); }} catch (e) {{}}
    location.reload();
  }}
  pfBtn.addEventListener('click', e => {{"""


def profile_painter(t):
    return f"""  function paintProfile() {{
    pfBtn.replaceChildren(...profileBits());
    pfBtn.setAttribute('aria-label', '{t["profile"]}: ' + PROFILE.name + ', ' + PROFILE.email);
    pfBtn.title = PROFILE.name + ' · ' + PROFILE.email;
    document.querySelector('.nav-profile')?.replaceChildren(...profileBits());
  }}
  paintProfile();
"""


def legal_addresses(t):
    return f"const LEGAL = {{ terms: '{t['terms'][1]}', privacy: '{t['privacy'][1]}' }};"


def session_expired(t):
    return f"if (code === 'unauthenticated') {{ setSync('err', '{t['session_expired']}'); showAuth(); }}\n            else if (code === 'invalid_argument') {{ readOnly = true;"


def per_language(name, old, make_new):
    """The same text appears once in the Polish app and once in the English one."""
    return name, old, [make_new(TEXT["pl"]), make_new(TEXT["en"])]


def once(name, old, new):
    return name, old, [new]


def both(name, old, new):
    return name, old, [new, new]


def single(name, pairs):
    """Text that differs between the languages: one (old, new) pair per language."""
    return [(f"{name} ({lang})", old, [new]) for lang, (old, new) in zip(("pl", "en"), pairs)]


CHANGES = [
    # Julia's export is the body of a Claude artifact; Claude adds the document skeleton when it publishes the page.
    # Everything of hers goes into <body>: the language loader looks for #markup-pl-start among the body's children
    # (without an explicit <body> the parser would leave that marker in <head>).
    once("document skeleton (charset, viewport)",
         "<title>Maiday</title>\n",
         '<!doctype html>\n<html lang="pl">\n<head>\n<meta charset="utf-8">\n'
         '<meta name="viewport" content="width=device-width, initial-scale=1">\n</head>\n<body>\n'
         "<title>Maiday</title>\n"),
    once("fonts served locally",
         '<link rel="preconnect" href="https://fonts.googleapis.com">\n'
         '<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Averia+Serif+Libre:wght@400;700&family=Onest:wght@300;400;500;600&family=JetBrains+Mono:wght@400;500&display=swap">',
         '<link rel="stylesheet" href="fonts/fonts.css">'),
    once("Lucide served locally",
         '<script src="https://cdn.jsdelivr.net/npm/lucide@1.48.0/dist/umd/lucide.min.js"></script>',
         '<script src="vendor/lucide-1.48.0.min.js"></script>'),
    once("sign-in background from this site (Julia's points at an asset of the Claude artifact)",
         'url("/_blob/6bc8030f0f4b61363069d3304a8a069a")',
         'url("auth-bg.webp")'),
    once("Google mark (style)",
         ".auth-g { display: inline-grid; } .auth-g svg { width: 18px; height: 18px; }",
         ".auth-g { display: inline-grid; place-items: center; } .auth-g svg { width: 20px; height: 20px; }"),
    both("inClaude()",
         "  async function connect() {",
         "  const inClaude = () => !!(window.claude && typeof window.claude.use === 'function');\n  async function connect() {"),
    both("outside Claude the data comes from the API",
         "    if (!window.claude || typeof window.claude.use !== 'function') return;",
         "    if (!inClaude()) return connectApi();"),
    per_language("API client", "    } catch (e) { /* stay on browser storage */ }\n  }\n",
                 lambda t: "    } catch (e) { /* stay on browser storage */ }\n  }\n" + api_client(t)),
    both("entries stay in the language they were written in (no translation outside Claude)",
         "const scheduleTr = () => { clearTimeout(trTimer); if (!trStop && !trBusy)",
         "const scheduleTr = () => { clearTimeout(trTimer); if (inClaude() && !trStop && !trBusy)"),
    per_language("expired session", "if (code === 'invalid_argument') { readOnly = true;", session_expired),
    both("profile from the account instead of the sample",
         "const PROFILE = { name: 'Julia Jędrzejczak', email: 'j.jedrzejczak@gmail.com' };",
         "const PROFILE = inClaude() ? { name: 'Julia Jędrzejczak', email: 'j.jedrzejczak@gmail.com' } : { name: '', email: '' };"),
    *single("profile painter", [
        ("  pfBtn.replaceChildren(...profileBits());\n"
         "  pfBtn.setAttribute('aria-label', 'Profil: ' + PROFILE.name + ', ' + PROFILE.email);\n"
         "  pfBtn.title = PROFILE.name + ' · ' + PROFILE.email;\n", profile_painter(TEXT["pl"])),
        ("  pfBtn.replaceChildren(...profileBits());\n"
         "  pfBtn.setAttribute('aria-label', 'Profile: ' + PROFILE.name + ', ' + PROFILE.email);\n"
         "  pfBtn.title = PROFILE.name + ' · ' + PROFILE.email;\n", profile_painter(TEXT["en"])),
    ]),
    per_language("logout, account deletion", "  pfBtn.addEventListener('click', e => {", account_actions),
    both("account deletion goes through the API",
         "      onConfirm: () => {\n        const ids = new Set();",
         "      onConfirm: () => {\n        if (!inClaude() && !TRIAL) return deleteAccountApi();\n        const ids = new Set();"),
    both("logout in the profile menu",
         "b.setAttribute('aria-expanded', 'false'); showAuth('login'); } }, logoutIcon(),",
         "b.setAttribute('aria-expanded', 'false'); logout(); } }, logoutIcon(),"),
    both("logout in the phone menu",
         "onclick: () => showAuth('login') }, logoutIcon(),",
         "onclick: logout }, logoutIcon(),"),
    per_language("legal documents (settings, sign-in screen)", "const LEGAL = { terms: '#', privacy: '#' };", legal_addresses),
    both("data migrations wait for the account's data (otherwise they run on the empty local state first)",
         "const dataReady = () => mode === 'local' || COLLS.every(c => HUB.ready && HUB.ready[c]);",
         "const dataReady = () => inClaude() || TRIAL ? mode === 'local' || COLLS.every(c => HUB.ready && HUB.ready[c]) : apiLoaded;"),
    both("Google sign-in",
         "  $('#auth-google').prepend(h('span', { class: 'auth-g', 'aria-hidden': 'true' }));\n"
         "  $('#auth-google').addEventListener('click', hideAuth);",
         f"  const GOOGLE_G = '{GOOGLE_G}';\n"
         "  { const g = h('span', { class: 'auth-g', 'aria-hidden': 'true' }); g.innerHTML = GOOGLE_G; $('#auth-google').prepend(g); }\n"
         "  $('#auth-google').addEventListener('click', () => { if (inClaude() || TRIAL) hideAuth(); else location.href = '/api/auth/login'; });"),
    *single("upload unavailable message", [
        ("'Dodawanie zdjęć i plików działa w zapisanej wersji Maiday w Claude.'", "'Dodawanie zdjęć i plików będzie dostępne wkrótce.'"),
        ("'Adding images and files works in the saved version of Maiday in Claude.'", "'Adding images and files will be available soon.'"),
    ]),
    *single("upload unavailable tooltip", [
        ("'Zdjęcia i pliki działają w zapisanej wersji Maiday w Claude'", "'Zdjęcia i pliki będą dostępne wkrótce'"),
        ("'Images and files work in the saved version of Maiday in Claude'", "'Images and files will be available soon'"),
    ]),
    both("cover upload tooltip (pl)", "'Wgrywanie działa w zapisanej wersji Maiday w Claude'", "'Wgrywanie zdjęć będzie dostępne wkrótce'"),
    both("cover upload tooltip (en)", "'Uploading works in the saved version of Maiday in Claude'", "'Uploading images will be available soon'"),
]


# Julia's demo link (maiday-trial.html) is the same app with the TRIAL flag on: no sign-in, data only in the browser.
# We build it from the same upstream file instead of keeping a second copy.
DEMO_CHANGES = [
    both("TRIAL flag", "  const TRIAL = false;", "  const TRIAL = true;"),
    both("separate local storage", "  const LS = 'maiday.v1';", "  const LS = 'maiday.trial';"),
    once("title", "<title>Maiday</title>", "<title>Maiday – demo</title>"),
]


def apply(html, changes):
    problems = []
    for name, old, news in changes:
        parts = html.split(old)
        if len(parts) - 1 != len(news):
            problems.append(f"  {name}: expected {len(news)}×, found {len(parts) - 1}×\n    {old.splitlines()[0][:120]}")
            continue
        html = parts[0] + "".join(new + rest for new, rest in zip(news, parts[1:]))
    if problems:
        sys.exit("Julia's version changed where these adaptations apply; update them in adapt.py:\n" + "\n".join(problems))
    return html


def adapt(html):
    if html.lstrip().lower().startswith("<!doctype"):
        sys.exit("Julia's file already has a document skeleton: remove the \"document skeleton\" change from adapt.py")
    return apply(html, CHANGES).rstrip("\n") + "\n</body>\n</html>\n"


def main():
    app = adapt(UPSTREAM.read_text(encoding="utf-8"))
    outputs = {PUBLIC: app, DEMO: apply(app, DEMO_CHANGES)}
    if "--check" in sys.argv:
        for path, built in outputs.items():
            if not path.exists() or path.read_text(encoding="utf-8") != built:
                sys.exit(f"{path.relative_to(WEB.parent.parent)} is out of date: run python3 app/web/adapt.py")
        return
    for path, built in outputs.items():
        path.write_text(built, encoding="utf-8")
    print(f"{PUBLIC.relative_to(WEB.parent.parent)}: {len(CHANGES)} changes applied; {DEMO.name}: + {len(DEMO_CHANGES)} for the demo")


if __name__ == "__main__":
    main()
