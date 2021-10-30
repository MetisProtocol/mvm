// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;
/* Contract Imports */
/* External Imports */
import { Ownable } from "@openzeppelin/contracts/access/Ownable.sol";
import { iMVM_DiscountOracle } from "./iMVM_DiscountOracle.sol";
import { MVM_AddressResolver } from "../libraries/resolver/MVM_AddressResolver.sol";
contract MVM_DiscountOracle is Ownable, iMVM_DiscountOracle, MVM_AddressResolver{
    // Current l2 gas price
    uint256 public discount;
    uint256 public minL2Gas;
    mapping (address => bool) public xDomainWL;
    bool allowAllXDomainSenders;
    constructor(
      address _mvmAddressManager,
      uint256 _initialDiscount
    )
      Ownable() 
      MVM_AddressResolver(_mvmAddressManager)
    {
      setDiscount(_initialDiscount);
      setMinL2Gas(1_000_000);
      allowAllXDomainSenders = false;
    }
    
    function getMinL2Gas() view public override returns (uint256){
      return minL2Gas;
    }
    
    function getDiscount() view public override returns (uint256){
      return discount;
    }
    
    

    function setDiscount(
        uint256 _discount
    )
        public
        override
        onlyOwner
    {
        discount = _discount;
    }
    
    function setMinL2Gas(
        uint256 _minL2Gas
    )
        public
        override
        onlyOwner
    {
        minL2Gas = _minL2Gas;
    }

    function transferSetter(address newsetter) public override onlyOwner{
       transferOwnership(newsetter);
    }
    
    function setWhitelistedXDomainSender(
        address _sender,
        bool _isWhitelisted
    )
        external
        override
        onlyOwner
    {
        xDomainWL[_sender] = _isWhitelisted;
    }
    
    function isXDomainSenderAllowed(
        address _sender
    )
        view
        override
        public
        returns (
            bool
        )
    {
        return (
            allowAllXDomainSenders == true
            || xDomainWL[_sender]
        );
    }
    
    function setAllowAllXDomainSenders(
        bool _allowAllXDomainSenders
    )
        public
        override
        onlyOwner
    {
        allowAllXDomainSenders = _allowAllXDomainSenders;
    }
    
    function processL2SeqGas(address sender, uint256 _chainId) 
    public payable override {
        require(isXDomainSenderAllowed(sender), "sender is not whitelisted");
        string memory ch = string(abi.encodePacked(uint2str(_chainId),"_MVM_Sequencer"));
        
        address sequencer = resolveFromMvm(ch);
        require (sequencer != address(0), string(abi.encodePacked("sequencer address not available: ", ch)));
        
        //take the fee
        (payable(sequencer)).transfer(msg.value);
    }
    
    function isTrustedRelayer(uint256 chainid, address sender) view public override returns(bool) {
        return (sender == resolveFromMvm(string(abi.encodePacked(uint2str(chainid), "_MVM_RELAYER"))));
    }

    function uint2str(uint _i) internal pure returns (string memory _uintAsString) {
        if (_i == 0) {
            return "0";
        }
        uint j = _i;
        uint len;
        while (j != 0) {
            len++;
            j /= 10;
        }
        bytes memory bstr = new bytes(len);
        uint k = len;
        while (_i != 0) {
            k = k-1;
            uint8 temp = (48 + uint8(_i - _i / 10 * 10));
            bytes1 b1 = bytes1(temp);
            bstr[k] = b1;
            _i /= 10;
        }
        return string(bstr);
    }
}
