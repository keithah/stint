CREATE TABLE IF NOT EXISTS program_languages (
  name text PRIMARY KEY,
  color text NOT NULL
);

INSERT INTO program_languages (name, color) VALUES
  ('Go', '#00ADD8'),
  ('TypeScript', '#3178C6'),
  ('JavaScript', '#F1E05A'),
  ('Python', '#3572A5'),
  ('Rust', '#DEA584'),
  ('Ruby', '#701516'),
  ('Java', '#B07219'),
  ('C', '#555555'),
  ('C++', '#F34B7D'),
  ('C#', '#178600'),
  ('PHP', '#4F5D95'),
  ('Swift', '#F05138'),
  ('Kotlin', '#A97BFF'),
  ('Dart', '#00B4AB'),
  ('Scala', '#C22D40'),
  ('Elixir', '#6E4A7E'),
  ('Erlang', '#B83998'),
  ('Clojure', '#DB5855'),
  ('F#', '#B845FC'),
  ('Haskell', '#5E5086'),
  ('Lua', '#000080'),
  ('R', '#198CE7'),
  ('Shell', '#89E051'),
  ('PowerShell', '#012456'),
  ('HTML', '#E34C26'),
  ('CSS', '#563D7C'),
  ('SCSS', '#C6538C'),
  ('SQL', '#E38C00'),
  ('GraphQL', '#E10098'),
  ('JSON', '#292929'),
  ('YAML', '#CB171E'),
  ('TOML', '#9C4221'),
  ('Markdown', '#083FA1'),
  ('Dockerfile', '#384D54'),
  ('Terraform', '#7B42BC'),
  ('Nix', '#7E7EFF'),
  ('Objective-C', '#438EFF'),
  ('Perl', '#0298C3'),
  ('Racket', '#3C5CAA'),
  ('Vim Script', '#199F4B'),
  ('Vue', '#41B883'),
  ('Svelte', '#FF3E00')
ON CONFLICT (name) DO UPDATE SET color = EXCLUDED.color;

CREATE TABLE IF NOT EXISTS editor_plugins (
  key text PRIMARY KEY,
  name text NOT NULL,
  version text
);

INSERT INTO editor_plugins (key, name, version) VALUES
  ('vscode', 'VS Code', '1.89+'),
  ('cursor', 'Cursor', '0.40+'),
  ('zed', 'Zed', '0.140+'),
  ('neovim', 'Neovim', '0.9+'),
  ('vim', 'Vim', '8+'),
  ('intellij', 'IntelliJ IDEA', '2024+'),
  ('goland', 'GoLand', '2024+'),
  ('pycharm', 'PyCharm', '2024+'),
  ('webstorm', 'WebStorm', '2024+'),
  ('sublime', 'Sublime Text', '4+'),
  ('emacs', 'Emacs', '29+'),
  ('xcode', 'Xcode', '15+'),
  ('androidstudio', 'Android Studio', '2024+')
ON CONFLICT (key) DO UPDATE SET name = EXCLUDED.name, version = EXCLUDED.version;
