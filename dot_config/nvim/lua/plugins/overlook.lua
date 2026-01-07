return {
    "WilliamHsieh/overlook.nvim",
    opts = {},
    keys = {
        { "<leader>pu", function() require("overlook.api").restore_popup() end, desc = "Restore last popup" },
        { "<leader>pU", function() require("overlook.api").restore_all_popups() end, desc = "Restore all popups" },
        { "<leader>pc", function() require("overlook.api").close_all() end, desc = "Close all popups" },
        { "<leader>ps", function() require("overlook.api").open_in_split() end, desc = "Open popup in split" },
        { "<leader>pv", function() require("overlook.api").open_in_vsplit() end, desc = "Open popup in vsplit" },
        { "<leader>pt", function() require("overlook.api").open_in_tab() end, desc = "Open popup in tab" },
        { "<leader>po", function() require("overlook.api").open_in_original_window() end, desc = "Open popup in current window" },
    },
    init = function()
        -- gd を overlook の peek_definition に置き換え（LspAttach 時に設定）
        vim.api.nvim_create_autocmd("LspAttach", {
            group = vim.api.nvim_create_augroup("overlook_gd_keymap", { clear = true }),
            callback = function(args)
                vim.keymap.set("n", "gd", function()
                    require("overlook.api").peek_definition()
                end, { buffer = args.buf, desc = "Goto Definition" })
            end,
        })

        -- ポップアップ内のキーマップ
        vim.api.nvim_create_autocmd("BufWinEnter", {
            group = vim.api.nvim_create_augroup("overlook_enter_mapping", { clear = true }),
            pattern = "*",
            callback = function()
                vim.schedule(function()
                    if vim.w.is_overlook_popup then
                        vim.keymap.set("n", "<CR>", function()
                            require("overlook.api").open_in_original_window()
                        end, { buffer = true, desc = "Overlook: Open in original window" })
                        for _, lhs in ipairs({ "<C-CR>", ";" }) do
                            vim.keymap.set("n", lhs, function()
                                require("overlook.api").open_in_vsplit()
                            end, { buffer = true, desc = "Overlook: Open in vertical split" })
                        end
                    end
                end)
            end,
        })
    end,
}
