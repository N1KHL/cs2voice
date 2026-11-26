# Translations

One JSON file per language, named by its language code (`en.json`, `pt-BR.json`, ...).

- `_name` is the name shown in the language picker.
- Keys missing from a file fall back to English (`en.json`).
- `{n}`, `{name}`, ... are filled in by the app; keep them.
- To add a language: copy `en.json`, translate the values, add the file name to
  `index.json`, rebuild.

The FACEIT report text is always in English (FACEIT support reads English).
