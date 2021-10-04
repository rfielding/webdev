package policy

Stat = true
Create = true
Read = true

Write{
    input.claims.groups.username[_] == "rob"
}
Delete{
    Write
}

Banner = "PRIVATE"
BannerForeground = "white"
BannerBackground = "red"
