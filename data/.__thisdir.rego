package policy

Read = true
Stat = true
Create{
    true
    #some i
    #input.claims.groups.username[_] == input.action.name[i] 
    #input.action.action[i] == "Create"
}
Banner = "PUBLIC"
BannerForeground = "white"
BannerBackground = "green"
