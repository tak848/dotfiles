set shell=/bin/zsh
set encoding=utf-8
lang en_US.UTF-8
set shiftwidth=4
set tabstop=4
set expandtab
set textwidth=0
set autoindent
set hlsearch
set clipboard=unnamed
set number
set history=10000
set wildmode=full
set nocompatible
filetype plugin on
runtime macros/matchit.vim

if executable('im-select')
autocmd InsertLeave * :call system('im-select com.apple.keylayout.ABC')
autocmd CmdlineLeave * :call system('im-select com.apple.keylayout.ABC')
endi

syntax on

call plug#begin()
Plug 'tpope/vim-surround'
call plug#end()

set smartcase
set ignorecase
