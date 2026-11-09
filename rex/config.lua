-- Loaded directly by Rex's init.lua; no plugin manager is required.
return function(options)
local binary = assert(options.bin, "Rex Review needs the installed binary path")

rex.action{
  name = "rex_review",
  title = "Rex Review: Git Changes",
  description = "Review changes and comment on lines for this pane's agent",
  args = { repo = "string?" },
  run = function(ctx, args)
    local view, err = rex.session.view{ session_id = ctx.session_id }
    if not view then error(err) end
    local origin
    for _, window in ipairs(view.windows or {}) do
      for _, layer in ipairs(window.layers or {}) do
        for _, block in ipairs(layer.blocks or {}) do
          if block.block_id == ctx.block_id then origin = window end
        end
      end
    end
    if not origin then error("Open Rex Review from an agent pane") end
    -- Toggle only this tab's review. The process saves drafts on every edit.
    for _, layer in ipairs(origin.layers or {}) do
      for _, block in ipairs(layer.blocks or {}) do
        if block.label == "rex-review" then
          local result, close_err = rex.session.close_layer{
            session_id = ctx.session_id, layer_id = layer.layer_id,
          }
          if not result then error(close_err) end
          return result
        end
      end
    end
    -- Server-created terminals otherwise begin with Rex's generic colors.
    -- Read local client preferences at each open, so the selected light/dark
    -- theme is applied before the review program renders its first frame.
    local terminal_options = { command = {
      binary, "--session", ctx.session_id,
      "--window", origin.window_id, "--from", ctx.block_id,
      "--server", options.server or "",
      "--repo", args.repo or "",
    } }
    local appearance = rex.exec({ binary, "theme" }, { timeout = 3 })
    if appearance and appearance.ok then
      local theme = rex.json.decode(appearance.stdout)
      if theme and theme.foreground and theme.background then terminal_options.theme = theme end
    end
    local result, create_err = rex.session.new_layer{
      session_id = ctx.session_id,
      window_id = origin.window_id,
      bounds = { 0.03, 0.04, 0.94, 0.92 },
      focus = true,
      layout = rex.layout.block{
        flavor = "com.superlogical.terminal.shell",
        label = "rex-review",
        options = terminal_options,
      },
    }
    if not result then error(create_err) end
    return result
  end,
}

rex.bind("cmd+shift+r", "rex_review")
end
