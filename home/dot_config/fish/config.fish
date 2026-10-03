# Homebrew (macOS on Apple silicon, or Linuxbrew)
if test -x /opt/homebrew/bin/brew
    /opt/homebrew/bin/brew shellenv fish | source
else if test -x /home/linuxbrew/.linuxbrew/bin/brew
    /home/linuxbrew/.linuxbrew/bin/brew shellenv fish | source
end

set -gx HOMEBREW_NO_ENV_HINTS 1
set -gx EDITOR zed
set -gx VISUAL zed
set -gx CLAUDE_STATUSLINE_GRADIENT 1

if test (uname) = Darwin
    set -gx PNPM_HOME "$HOME/Library/pnpm"
else
    set -gx PNPM_HOME "$HOME/.local/share/pnpm"
end

# OrbStack: command-line tools and integration
if test -f ~/.orbstack/shell/init2.fish
    source ~/.orbstack/shell/init2.fish
end

# uv, codebase-memory-mcp, and other user tools
fish_add_path "$HOME/.local/bin"

# pnpm
if not contains -- $PNPM_HOME $PATH
    set -gx PATH "$PNPM_HOME" $PATH
end

# conda hook (miniforge). Not `conda init fish`, which writes absolute paths.
# Before mise, so the mise shims still win.
for base in /opt/homebrew/Caskroom/miniforge/base /usr/local/Caskroom/miniforge/base "$HOME/miniforge3"
    if test -x $base/bin/conda
        eval $base/bin/conda shell.fish hook 2>/dev/null | source
        break
    end
end

# mise shims (must come after PNPM_HOME so it takes precedence)
fish_add_path --global "$HOME/.local/share/mise/shims"

if status is-interactive
    if command -q mise
        mise activate fish | source
    end
    if command -q starship
        starship init fish | source
    end
end
