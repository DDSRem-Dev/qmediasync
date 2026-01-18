set version=%1
git push origin main
git tag %version%
git push origin --tags