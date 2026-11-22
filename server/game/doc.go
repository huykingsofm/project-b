// Package game is the game that runs on the platform: it gives the states of the world, the zones
// and the zonelayers, and the actions and the commands that change them, and builds the cluster of
// engines that runs them. The platform (package platform) knows nothing of what is here; this
// package imports it, and cmd wires the two.
package game
