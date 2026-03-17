package net.demimine.dynamic;

public final class BuildConstants {
    public static final String VERSION = "${version}";
    private BuildConstants() {
        throw new UnsupportedOperationException("This is a utility class and cannot be instantiated");
    }
}
