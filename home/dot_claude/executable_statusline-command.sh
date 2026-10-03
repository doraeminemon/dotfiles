#!/bin/bash
# Claude Code statusLine script
# Segments: directory | git branch | model | effort | context usage | token usage | 5h limit | 7d limit

input=$(cat)

cwd=$(printf '%s' "$input" | jq -r '.workspace.current_dir // .cwd // empty')
model=$(printf '%s' "$input" | jq -r '.model.display_name // "unknown"')
effort=$(printf '%s' "$input" | jq -r '.effort.level // empty')
used_pct=$(printf '%s' "$input" | jq -r '.context_window.used_percentage // empty')
in_tok=$(printf '%s' "$input" | jq -r '.context_window.total_input_tokens // empty')
out_tok=$(printf '%s' "$input" | jq -r '.context_window.total_output_tokens // empty')
five_h_pct=$(printf '%s' "$input" | jq -r '.rate_limits.five_hour.used_percentage // empty')
seven_d_pct=$(printf '%s' "$input" | jq -r '.rate_limits.seven_day.used_percentage // empty')

dir_display="${cwd/#$HOME/\~}"
dir_display=$(basename "$dir_display")
[ "$dir_display" = "~" ] && dir_display="~"

# Git branch (skip optional locks so we never block on a git lock file)
branch=""
if [ -n "$cwd" ] && git -C "$cwd" --no-optional-locks rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  branch=$(git -C "$cwd" --no-optional-locks branch --show-current 2>/dev/null)
  if [ -z "$branch" ]; then
    branch=$(git -C "$cwd" --no-optional-locks rev-parse --short HEAD 2>/dev/null)
  fi
fi

format_tokens() {
  awk -v n="$1" 'BEGIN {
    if (n >= 1000000) printf "%.1fM", n/1000000;
    else if (n >= 1000) printf "%.1fk", n/1000;
    else printf "%d", n+0;
  }'
}

# Renders a percentage as a fixed-width block-character progress bar in the
# style of Charm's Bubble Tea `bubbles/progress` component: no bracket
# wrapping, and the filled portion is a smooth 24-bit truecolor gradient
# (interpolated per-character via \033[38;2;R;G;Bm), rather than one flat
# color. Since these bars represent consumption meters (context %, rate
# limits), the gradient runs low-usage green -> high-usage red instead of
# Bubbles' default decorative purple, so color itself carries meaning. The
# unfilled portion stays a flat dim gray (same tone as C_SEP) so the
# gradient reads clearly against it. Shared by the context-usage and both
# rate-limit segments so they all share one visual style/width.
BAR_WIDTH=10
render_bar() {
  local pct="$1"
  local width="$BAR_WIDTH"
  local filled empty bar i r g b
  filled=$(awk -v p="$pct" -v w="$width" 'BEGIN {
    f = int((p / 100) * w + 0.5);
    if (f > w) f = w;
    if (f < 0) f = 0;
    print f;
  }')
  empty=$((width - filled))
  bar=""
  for ((i = 0; i < filled; i++)); do
    # Interpolate this cell's color by its position along the *full* bar
    # width (not just the filled span), so the gradient always represents
    # the same fixed green->red scale no matter how full the bar currently is.
    read -r r g b <<<"$(awk -v i="$i" -v w="$width" \
      -v r0="$GRAD_R0" -v g0="$GRAD_G0" -v b0="$GRAD_B0" \
      -v r1="$GRAD_R1" -v g1="$GRAD_G1" -v b1="$GRAD_B1" 'BEGIN {
        t = (w > 1) ? i / (w - 1) : 0;
        printf "%d %d %d", r0 + (r1 - r0) * t + 0.5, g0 + (g1 - g0) * t + 0.5, b0 + (b1 - b0) * t + 0.5;
      }')"
    bar+="\033[38;2;${r};${g};${b}m█"
  done
  if ((empty > 0)); then
    bar+="${C_BAR_EMPTY}"
    for ((i = 0; i < empty; i++)); do bar+="░"; done
  fi
  bar+="${C_RESET}"
  printf '%s' "$bar"
}

# ANSI colors matched to the user's Starship theme (~/.config/starship.toml).
# That config only overrides module *symbols*, not colors, so it runs on
# Starship's built-in default style. We reuse those defaults where a module
# maps directly, and pick the closest default-palette accent for segments
# Starship has no equivalent module for:
#   - directory   -> Starship `directory` default style: bold cyan
#   - git branch  -> Starship `git_branch` default style: bold purple
#   - model       -> approximated using Starship `username`/`cmd_duration` default: bold yellow
#   - effort      -> approximated using Starship `package` default: bold orange (256-color 208)
#   - context %   -> approximated using Starship `memory_usage` default: bold dimmed white
#   - tokens      -> generic dim gray (matches Starship's muted/secondary text)
#   - rate limits -> approximated using Starship `battery` default: bold red (usage/consumption meter)
C_DIR='\033[1;36m'          # bold cyan   (directory)
C_GIT='\033[1;35m'          # bold purple (git_branch)
C_MODEL='\033[1;33m'        # bold yellow (username/cmd_duration)
C_EFFORT='\033[1;38;5;208m' # bold orange (package)
C_CTX='\033[1;2;37m'        # bold dimmed white (memory_usage)
C_TOK='\033[90m'            # gray (secondary/muted text)
C_LIMIT='\033[1;31m'        # bold red    (battery)
C_SEP='\033[90m'            # gray
C_RESET='\033[0m'

# render_bar() gradient anchors: low-usage green (#2ecc40) -> high-usage red
# (Bootstrap "danger" #dc3545), 24-bit truecolor. The unfilled track uses the
# same dim gray as C_SEP/C_TOK so the gradient stays the visual focal point.
GRAD_R0=46  GRAD_G0=204 GRAD_B0=64  # green anchor (0% along the bar)
GRAD_R1=220 GRAD_G1=53  GRAD_B1=69  # red anchor   (100% along the bar)
C_BAR_EMPTY='\033[90m'              # dim gray track (matches C_SEP)

SEP="${C_SEP} | ${C_RESET}"

segments=()

[ -n "$dir_display" ] && segments+=("${C_DIR}${dir_display}${C_RESET}")
[ -n "$branch" ] && segments+=("${C_GIT}\xe2\x8e\x87 ${branch}${C_RESET}")
segments+=("${C_MODEL}${model}${C_RESET}")
[ -n "$effort" ] && segments+=("${C_EFFORT}effort:${effort}${C_RESET}")

if [ -n "$used_pct" ]; then
  pct_rounded=$(awk -v p="$used_pct" 'BEGIN { printf "%.0f", p }')
  segments+=("${C_CTX}ctx:$(render_bar "$used_pct")${C_CTX} ${pct_rounded}%${C_RESET}")
fi

if [ -n "$in_tok" ] && [ -n "$out_tok" ]; then
  total_tok=$((in_tok + out_tok))
  segments+=("${C_TOK}tok:$(format_tokens "$total_tok")${C_RESET}")
fi

# Claude.ai/Claude Code subscription usage limits (5-hour rolling window and
# 7-day weekly window). Only present for subscribers after the first API
# response of the session, so each is its own independently optional segment,
# same as effort/context above.
if [ -n "$five_h_pct" ]; then
  five_h_rounded=$(awk -v p="$five_h_pct" 'BEGIN { printf "%.0f", p }')
  segments+=("${C_LIMIT}5h:$(render_bar "$five_h_pct")${C_LIMIT} ${five_h_rounded}%${C_RESET}")
fi

if [ -n "$seven_d_pct" ]; then
  seven_d_rounded=$(awk -v p="$seven_d_pct" 'BEGIN { printf "%.0f", p }')
  segments+=("${C_LIMIT}7d:$(render_bar "$seven_d_pct")${C_LIMIT} ${seven_d_rounded}%${C_RESET}")
fi

out=""
first=1
for seg in "${segments[@]}"; do
  if [ "$first" -eq 1 ]; then
    out="$seg"
    first=0
  else
    out="${out}${SEP}${seg}"
  fi
done

printf "%b\n" "$out"
