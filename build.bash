#!/bin/bash
pushd bin
xgo -out pnx --targets=windows/*,darwin/* github.com/oov/pnx
popd
