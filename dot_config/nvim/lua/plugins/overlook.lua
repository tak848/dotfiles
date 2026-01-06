return {
    "WilliamHsieh/overlook.nvim",
    opts = {},
    keys = {
        { "<leader>pd", function() require("overlook.api").peek_definition() end, desc = "Peek definition" },
        { "<leader>pc", function() require("overlook.api").close_all() end, desc = "Close all popup" },
        { "<leader>pu", function() require("overlook.api").restore_popup() end, desc = "Restore popup" },
        { "<leader>pU", function() require("overlook.api").restore_all_popups() end, desc = "Restore all popups" },
        { "<leader>ps", function() require("overlook.api").open_in_split() end, desc = "Open in split" },
        { "<leader>pv", function() require("overlook.api").open_in_vsplit() end, desc = "Open in vsplit" },
        { "<leader>pt", function() require("overlook.api").open_in_tab() end, desc = "Open in tab" },
        { "<leader>po", function() require("overlook.api").open_in_original_window() end, desc = "Open in window" },
    },
    init = function()
        vim.api.nvim_create_autocmd("BufWinEnter", {
            callback = function()
                local dominated = require("overlook.api").is_dominated_win()
                if dominated then
                    vim.keymap.set("n", "<CR>", function()
                        require("overlook.api").open_in_original_window()
                    end, { buffer = true, desc = "Open in window" })
                    vim.keymap.set("n", "<C-CR>", function()
                        require("overlook.api").open_in_split()
                    end, { buffer = true, desc = "Open in split" })
                end
            end,
        })
    end,
}
