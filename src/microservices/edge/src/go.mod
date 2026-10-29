module gameflex/edge

go 1.23

require gameflex/webpage v0.0.0

// La page construite, servie avec le même code que le kernel du téléphone.
replace gameflex/webpage => /shared/webpage
