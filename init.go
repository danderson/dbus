package dbus

func init() {
	RegisterSignalType[NameOwnerChanged]("org.freedesktop.DBus", "NameOwnerChanged")
	RegisterSignalType[NameLost]("org.freedesktop.DBus", "NameLost")
	RegisterSignalType[NameAcquired]("org.freedesktop.DBus", "NameAcquired")
	RegisterSignalType[ActivatableServicesChanged]("org.freedesktop.DBus", "ActivatableServicesChanged")

	RegisterSignalType[PropertiesChanged]("org.freedesktop.DBus.Properties", "PropertiesChanged")

	RegisterSignalType[InterfacesAdded]("org.freedesktop.DBus.ObjectManager", "InterfacesAdded")
	RegisterSignalType[InterfacesRemoved]("org.freedesktop.DBus.ObjectManager", "InterfacesRemoved")
}
