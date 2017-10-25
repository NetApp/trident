# To build this, you need Python and the following:
#   python -m pip install sphinx sphinx-autobuild sphinx_rtd_theme

SPHINXOPTS    =
SPHINXBUILD   = sphinx-build
SPHINXPROJ    = Trident
SOURCEDIR     = .
BUILDDIR      = _build

# Put it first so that "make" without argument is like "make help".
help:
	@$(SPHINXBUILD) -M help "$(SOURCEDIR)" "$(BUILDDIR)" $(SPHINXOPTS) $(O)
	@echo Hey! Try \'make livehtml\'!

# Serves the docs on a webserver that updates as changes are made.
livehtml:
	sphinx-autobuild -b html $(SOURCEDIR) $(BUILDDIR)/html

.PHONY: help livehtml Makefile

# Catch-all target: route all unknown targets to Sphinx using the new
# "make mode" option.  $(O) is meant as a shortcut for $(SPHINXOPTS).
%: Makefile
	@$(SPHINXBUILD) -M $@ "$(SOURCEDIR)" "$(BUILDDIR)" $(SPHINXOPTS) $(O)
