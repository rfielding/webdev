package policy

Stat = true
Read = true

Write{
    input.claims.groups.username[_] == "jp"
}
Delete{
    Write
}
Create {
    Write	
}

Banner = "PRIVATE"
BannerForeground = "white"
BannerBackground = "red"
